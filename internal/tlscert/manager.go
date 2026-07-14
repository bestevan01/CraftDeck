// Package tlscert implements requirements.md's FR-33/33a: automatic HTTPS
// for the management web UI the moment WAN exposure is turned on -- a real,
// auto-renewed Let's Encrypt certificate via Cloudflare DNS-01 (reusing the
// same API token FR-31 already verifies and stores for a registered main
// domain, so no extra port 80/443 forwarding is needed for HTTP-01/TLS-ALPN-01
// the way the original FR-33a wording assumed) when a main domain is
// registered, or a self-signed fallback (selfsigned.go) when it isn't.
package tlscert

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"craftdeck/internal/ddns"
	"craftdeck/internal/secrets"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/cloudflare"
)

// Manager backs the web UI's tls.Config.GetCertificate: it looks at
// whatever domain is currently registered (internal/ddns) on every TLS
// handshake and serves the right kind of certificate for that state,
// switching automatically as the operator registers/unregisters a domain --
// no restart required.
type Manager struct {
	domains   *ddns.Repository
	masterKey []byte
	dataDir   string

	mu           sync.Mutex
	magic        *certmagic.Config // non-nil once a main_domain has been configured at least once
	managedFor   string            // the domain magic is currently configured/managing for
	managingErr  error             // last ManageAsync error for managedFor, surfaced instead of silently falling back
}

func NewManager(domains *ddns.Repository, masterKey []byte, dataDir string) *Manager {
	return &Manager{domains: domains, masterKey: masterKey, dataDir: dataDir}
}

// GetCertificate is a tls.Config.GetCertificate implementation: real
// Let's Encrypt cert (via certmagic) when a main domain is registered and
// the handshake's SNI is actually that domain, self-signed otherwise (any
// other hostname, or a bare-IP connection with no SNI at all -- confirmed
// on real hardware that skipping this check makes every plain IP:port
// connection attempt (and, worse, retry) a real ACME issuance for the
// registered domain, wasting Let's Encrypt's rate-limited quota for
// something that was never going to serve that domain's cert anyway).
// Errors from the real-cert path (e.g. a first issuance still in flight, or
// Cloudflare/ACME being unreachable) fall back to self-signed rather than
// failing the handshake outright -- a warned-but-working connection beats
// none at all.
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	ctx := context.Background()
	config, err := m.domains.Get(ctx)
	if err == nil && config != nil && config.Kind == ddns.KindMainDomain && config.ZoneID != "" &&
		config.TokenEncrypted != "" && strings.EqualFold(hello.ServerName, config.Hostname) {
		magic, syncErr := m.ensureManaged(ctx, config)
		if syncErr == nil {
			if cert, err := magic.GetCertificate(hello); err == nil {
				return cert, nil
			} else {
				log.Printf("tlscert: real certificate not ready yet for %s, serving self-signed instead: %v", config.Hostname, err)
			}
		} else {
			log.Printf("tlscert: couldn't configure certificate management for %s, serving self-signed instead: %v", config.Hostname, syncErr)
		}
	}
	return GetSelfSigned()
}

// ensureManaged (re)builds the certmagic config and starts background
// management the first time a given domain is seen, or whenever the
// registered domain changes (e.g. operator switches to a different main
// domain, or rotates the Cloudflare token by re-registering) -- ManageAsync
// itself is a no-op on repeat calls for a domain already under management,
// but a changed token means a whole new DNS01Solver/Config is needed since
// certmagic.Config isn't mutable after construction.
func (m *Manager) ensureManaged(ctx context.Context, config *ddns.Config) (*certmagic.Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.magic != nil && m.managedFor == config.Hostname && m.managingErr == nil {
		return m.magic, nil
	}

	token, err := secrets.Decrypt(m.masterKey, config.TokenEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt cloudflare token: %w", err)
	}

	cache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(certmagic.Certificate) (*certmagic.Config, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			return m.magic, nil
		},
	})
	configID := config.ID
	magic := certmagic.New(cache, certmagic.Config{
		Storage: &certmagic.FileStorage{Path: filepath.Join(m.dataDir, "certmagic")},
		// FR-33a: certmagic renews certificates on its own background
		// schedule (see "started background certificate maintenance" in the
		// daemon's logs) with no way for GetCertificate's caller to observe
		// a renewal that happened -- or failed -- outside of a live TLS
		// handshake. OnEvent is the one hook certmagic offers for exactly
		// this, emitting "cert_failed" (obtain or renewal, either one) and
		// "cert_obtained" -- recording the failure here (surfaced on the
		// "도메인 연결" card, same place as the free-subdomain mismatch
		// warning) is what actually lets an operator find out their
		// certificate is failing to renew before it expires, rather than
		// only when a browser starts rejecting it.
		OnEvent: func(ctx context.Context, event string, data map[string]any) error {
			switch event {
			case "cert_failed":
				msg := "unknown error"
				if errVal, ok := data["error"]; ok {
					msg = fmt.Sprint(errVal)
				}
				if err := m.domains.SetCertRenewalError(ctx, configID, msg); err != nil {
					log.Printf("tlscert: record cert renewal failure: %v", err)
				}
			case "cert_obtained":
				if err := m.domains.ClearCertRenewalError(ctx, configID); err != nil {
					log.Printf("tlscert: clear cert renewal failure: %v", err)
				}
			}
			return nil
		},
	})
	issuer := certmagic.NewACMEIssuer(magic, certmagic.ACMEIssuer{
		Agreed:                  true,
		DisableHTTPChallenge:    true,
		DisableTLSALPNChallenge: true,
		DNS01Solver: &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: &cloudflare.Provider{APIToken: token},
			},
		},
	})
	magic.Issuers = []certmagic.Issuer{issuer}

	m.magic = magic
	m.managedFor = config.Hostname
	m.managingErr = nil

	// ManageSync, not ManageAsync: this runs synchronously inside a live TLS
	// handshake (GetCertificate), so the caller is already blocked waiting
	// on us -- Async kicks off management in the background and returns
	// immediately, which left the very first handshake after every process
	// restart falling back to self-signed even when a valid certificate
	// already existed on disk (confirmed on real hardware), simply because
	// nothing had loaded it into the in-memory cache yet. Sync blocks until
	// the existing certificate is loaded (fast) or, the very first time
	// ever, a new one is actually obtained via ACME (slow, ~10s) -- an
	// acceptable one-time cost for an admin panel, and errors still fall
	// through to the self-signed fallback in GetCertificate either way.
	if err := magic.ManageSync(ctx, []string{config.Hostname}); err != nil {
		m.managingErr = err
		return nil, err
	}
	return magic, nil
}
