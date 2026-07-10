package instance

import (
	"context"
	"database/sql"
	"fmt"
)

// ProxyBackend mirrors one row of the `proxy_backends` table (see
// internal/db/migrations/0001_init.sql): one backend server a proxy
// instance (Velocity/BungeeCord) routes players to.
type ProxyBackend struct {
	ProxyID           string `json:"proxy_id"`
	BackendInstanceID string `json:"backend_instance_id"`
	Priority          int    `json:"priority"`
	ForcedHost        string `json:"forced_host,omitempty"`
}

func (r *Repository) ListProxyBackends(ctx context.Context, proxyID string) ([]*ProxyBackend, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT proxy_id, backend_instance_id, priority, forced_host
		FROM proxy_backends WHERE proxy_id = ? ORDER BY priority`, proxyID)
	if err != nil {
		return nil, fmt.Errorf("list proxy backends: %w", err)
	}
	defer rows.Close()

	out := []*ProxyBackend{} // never nil: frontend gets `[]`, not `null`, when empty
	for rows.Next() {
		var b ProxyBackend
		var forcedHost sql.NullString
		if err := rows.Scan(&b.ProxyID, &b.BackendInstanceID, &b.Priority, &forcedHost); err != nil {
			return nil, err
		}
		b.ForcedHost = forcedHost.String
		out = append(out, &b)
	}
	return out, rows.Err()
}

// SetProxyBackends replaces every backend assignment for proxyID with the
// given list in one transaction (the caller is expected to have already
// regenerated the proxy's on-disk config to match before/after calling
// this -- see handlers_proxy.go).
func (r *Repository) SetProxyBackends(ctx context.Context, proxyID string, backends []*ProxyBackend) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if _, err := tx.ExecContext(ctx, `DELETE FROM proxy_backends WHERE proxy_id = ?`, proxyID); err != nil {
		return fmt.Errorf("clear proxy backends: %w", err)
	}
	for _, b := range backends {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO proxy_backends (proxy_id, backend_instance_id, priority, forced_host)
			VALUES (?, ?, ?, ?)`, proxyID, b.BackendInstanceID, b.Priority, b.ForcedHost); err != nil {
			return fmt.Errorf("insert proxy backend %s: %w", b.BackendInstanceID, err)
		}
	}
	return tx.Commit()
}
