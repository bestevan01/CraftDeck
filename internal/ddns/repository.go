// Package ddns implements requirements.md's FR-26~31 (domain connection):
// the free-subdomain DDNS path (FR-26a's stated order, DuckDNS then ipTime
// -- see duckdns.go/monitor.go), the shared bookkeeping (FR-1f's "is an
// owned domain registered" gate), and FR-31's ownership verification for
// the owned main-domain path (see internal/dns.VerifyZoneAccess, called
// from handlers_domain.go before Set persists a main_domain registration).
// ZoneID is cached here specifically so FR-28/29/30's actual DNS-record
// automation (internal/dns.UpsertARecord/UpsertAAAARecord/UpsertSRVRecord,
// driven from internal/api/handlers_proxy.go's SyncMainDomainDNS) doesn't
// need to re-resolve which zone a domain belongs to on every call.
package ddns

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Kind string

const (
	KindFreeSubdomain Kind = "free_subdomain"
	KindMainDomain    Kind = "main_domain"
)

type Mode string

const (
	// ModeActive means CraftDeck itself pushes IP updates to the provider
	// (FR-26c) -- DuckDNS today.
	ModeActive Mode = "active"
	// ModeMonitor means CraftDeck can't actively renew this hostname
	// (FR-26b/e) -- ipTime today -- and instead just periodically checks
	// whether it still resolves to the router's current WAN IP (FR-26f).
	ModeMonitor Mode = "monitor"
)

// Config mirrors the `ddns_configs` table (see
// internal/db/migrations/0001_init.sql, 0004_ddns_monitor.sql). Only one
// row ever exists at a time -- requirements.md's FR-26 intro: "사용자는 두
// 가지 도메인 연결 방식 중 하나를 선택할 수 있으며" ("the operator picks one
// of the two connection methods"), so registering a new one replaces
// whatever was there before rather than accumulating a list.
type Config struct {
	ID       string `json:"id"`
	Kind     Kind   `json:"kind"`
	Provider string `json:"provider"`
	Hostname string `json:"hostname"`
	Mode     Mode   `json:"mode"`
	// TokenEncrypted is the provider API credential (DuckDNS's token), AES-
	// GCM sealed with the daemon's master key (internal/secrets) -- never
	// serialized back to the frontend and only decrypted momentarily by
	// Manager.Reconcile right before an actual provider API call.
	TokenEncrypted string `json:"-"`
	// ZoneID is the Cloudflare zone ID VerifyZoneAccess resolved for a
	// main_domain registration (FR-31) -- empty for free_subdomain, which
	// has no such provider-side concept. Never serialized to the frontend;
	// it's an internal handle for FR-28/29/30's record-management calls.
	ZoneID           string `json:"-"`
	LastKnownIP      string `json:"last_known_ip,omitempty"`
	LastCheckedAt    string `json:"last_checked_at,omitempty"`
	MismatchDetected bool   `json:"mismatch_detected"`
	// CertRenewalError/At implement FR-33a's pre-expiry warning: set by
	// internal/tlscert.Manager whenever certmagic reports a failed
	// obtain/renewal attempt for this main_domain (see its OnEvent hook),
	// cleared the next time one succeeds. Only ever populated for
	// kind=main_domain -- the only path with a real managed certificate.
	CertRenewalError   string `json:"cert_renewal_error,omitempty"`
	CertRenewalErrorAt string `json:"cert_renewal_error_at,omitempty"`
	CreatedAt          string `json:"created_at"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Get returns the current domain configuration, or nil if none is
// registered.
func (r *Repository) Get(ctx context.Context) (*Config, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, kind, provider, hostname, mode, credentials_encrypted,
		       last_known_ip, last_checked_at, mismatch_detected, created_at, zone_id,
		       cert_renewal_error, cert_renewal_error_at
		FROM ddns_configs LIMIT 1`)
	var c Config
	var tokenEncrypted, lastKnownIP, lastCheckedAt, zoneID, certErr, certErrAt sql.NullString
	if err := row.Scan(&c.ID, &c.Kind, &c.Provider, &c.Hostname, &c.Mode, &tokenEncrypted,
		&lastKnownIP, &lastCheckedAt, &c.MismatchDetected, &c.CreatedAt, &zoneID,
		&certErr, &certErrAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	c.TokenEncrypted = tokenEncrypted.String
	c.LastKnownIP = lastKnownIP.String
	c.LastCheckedAt = lastCheckedAt.String
	c.ZoneID = zoneID.String
	c.CertRenewalError = certErr.String
	c.CertRenewalErrorAt = certErrAt.String
	return &c, nil
}

// HasMainDomain reports whether the registered config (if any) is a
// user-owned domain -- the condition FR-1f gates Velocity's default
// multi-server proxy behavior on, since a free-subdomain provider can only
// ever point at one server (FR-27) and forced-host subdomain routing needs
// real DNS to be reachable at all.
func (r *Repository) HasMainDomain(ctx context.Context) (bool, error) {
	config, err := r.Get(ctx)
	if err != nil {
		return false, err
	}
	return config != nil && config.Kind == KindMainDomain, nil
}

// Set replaces whatever domain configuration exists (if any) with a new
// one -- see the Config doc comment for why this is a replace, not an
// insert. tokenEncrypted is empty for anything that doesn't need one (a
// monitor-only free-subdomain provider like ipTime that has no API at
// all). zoneID is the Cloudflare zone ID VerifyZoneAccess resolved (FR-31)
// -- empty for free_subdomain.
func (r *Repository) Set(ctx context.Context, kind Kind, provider, hostname string, mode Mode, tokenEncrypted, zoneID string) (*Config, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if _, err := tx.ExecContext(ctx, `DELETE FROM ddns_configs`); err != nil {
		return nil, fmt.Errorf("clear existing domain config: %w", err)
	}
	id := uuid.NewString()
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ddns_configs (id, kind, provider, hostname, mode, credentials_encrypted, created_at, zone_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, kind, provider, hostname, mode, nullIfEmpty(tokenEncrypted), createdAt, nullIfEmpty(zoneID)); err != nil {
		return nil, fmt.Errorf("insert domain config: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Config{
		ID: id, Kind: kind, Provider: provider, Hostname: hostname, Mode: mode,
		TokenEncrypted: tokenEncrypted, ZoneID: zoneID, CreatedAt: createdAt,
	}, nil
}

// Clear removes the current domain configuration, if any.
func (r *Repository) Clear(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM ddns_configs`)
	return err
}

// UpdateCheckResult records the outcome of the most recent reconcile pass
// (Manager.Reconcile) -- either a successful active-renewal push or a
// monitor-only resolution check (FR-26f).
func (r *Repository) UpdateCheckResult(ctx context.Context, id, resolvedIP string, mismatch bool) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE ddns_configs
		SET last_known_ip = ?, last_checked_at = ?, mismatch_detected = ?
		WHERE id = ?`,
		resolvedIP, time.Now().UTC().Format(time.RFC3339), mismatch, id)
	return err
}

// SetCertRenewalError records a certmagic obtain/renewal failure (FR-33a)
// for the domain config identified by id -- a no-op if that id is no
// longer the current registration (e.g. the operator swapped domains
// between the failure and this call), since there's nothing meaningful
// left to warn about for a domain that isn't registered anymore.
func (r *Repository) SetCertRenewalError(ctx context.Context, id, message string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE ddns_configs SET cert_renewal_error = ?, cert_renewal_error_at = ? WHERE id = ?`,
		message, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// ClearCertRenewalError clears a previously recorded failure once a
// subsequent obtain/renewal succeeds.
func (r *Repository) ClearCertRenewalError(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE ddns_configs SET cert_renewal_error = NULL, cert_renewal_error_at = NULL WHERE id = ?`, id)
	return err
}

func nullIfEmpty(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
