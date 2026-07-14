package ddns

import (
	"context"
	"fmt"
	"log"
	"time"

	"craftdeck/internal/network"
	"craftdeck/internal/secrets"
)

// ReconcileInterval is how often the background loop re-checks/renews the
// registered free-subdomain hostname against the current public IP
// (FR-30).
const ReconcileInterval = 20 * time.Minute

// Manager runs FR-26/30's periodic reconciliation: for a free-subdomain
// registration, push a fresh IP to an active-renewal provider or check a
// monitor-only provider's hostname for drift (FR-26f); for a main_domain
// registration, re-sync every forced-host subdomain's Cloudflare A record
// against the router's current public IP (FR-28/30) via mainDomainSync,
// which internal/api wires up to Server.SyncMainDomainDNS -- this package
// can't import internal/api directly (internal/api already imports this
// package), so the actual DNS-record logic lives there and is injected here
// as a callback instead.
type Manager struct {
	repo      *Repository
	masterKey []byte

	mainDomainSync func(ctx context.Context) error
}

func NewManager(repo *Repository, masterKey []byte) *Manager {
	return &Manager{repo: repo, masterKey: masterKey}
}

// SetMainDomainSync wires up the main_domain half of Reconcile (FR-28/30).
// Called once from cmd/craftdeckd/main.go after the api.Server exists --
// NewManager itself runs before that (api.NewServer takes the *Manager as a
// constructor argument), so the callback can't be passed in at
// construction time.
func (m *Manager) SetMainDomainSync(fn func(ctx context.Context) error) {
	m.mainDomainSync = fn
}

// Reconcile runs one pass immediately (used both by the timer loop in
// Start and by handleSetDomainSettings right after registering, so the
// operator gets instant feedback instead of waiting up to
// ReconcileInterval).
func (m *Manager) Reconcile(ctx context.Context) error {
	config, err := m.repo.Get(ctx)
	if err != nil {
		return err
	}
	if config == nil {
		return nil // nothing registered
	}
	if config.Kind == KindMainDomain {
		if m.mainDomainSync == nil {
			return nil
		}
		return m.mainDomainSync(ctx)
	}

	publicIP, err := network.FetchPublicIP(ctx)
	if err != nil {
		return fmt.Errorf("fetch public ip: %w", err)
	}
	// Best-effort: most home connections have no public IPv6 at all (FR-30's
	// AAAA requirement only applies when there is one to report -- see
	// network.FetchPublicIPv6's doc comment).
	publicIPv6 := ""
	if ip, err := network.FetchPublicIPv6(ctx); err == nil {
		publicIPv6 = ip
	}

	if IsMonitorOnly(config.Provider) {
		resolvedIP, mismatch, err := CheckMismatch(ctx, config.Hostname, publicIP)
		if err != nil {
			return err
		}
		return m.repo.UpdateCheckResult(ctx, config.ID, resolvedIP, mismatch)
	}

	updater, ok := GetUpdater(config.Provider)
	if !ok {
		return fmt.Errorf("no updater registered for provider %q", config.Provider)
	}
	token := ""
	if config.TokenEncrypted != "" {
		token, err = secrets.Decrypt(m.masterKey, config.TokenEncrypted)
		if err != nil {
			return fmt.Errorf("decrypt provider token: %w", err)
		}
	}
	if err := updater.Update(ctx, config.Hostname, token, publicIP, publicIPv6); err != nil {
		return err
	}
	return m.repo.UpdateCheckResult(ctx, config.ID, publicIP, false)
}

// Start runs Reconcile once immediately, then on ReconcileInterval, until
// ctx is canceled. Errors are logged, not returned -- a transient failure
// (provider API down, DNS resolution hiccup) shouldn't stop future
// attempts.
func (m *Manager) Start(ctx context.Context) {
	if err := m.Reconcile(ctx); err != nil {
		log.Printf("ddns: initial reconcile: %v", err)
	}
	go func() {
		ticker := time.NewTicker(ReconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.Reconcile(ctx); err != nil {
					log.Printf("ddns: reconcile: %v", err)
				}
			}
		}
	}()
}
