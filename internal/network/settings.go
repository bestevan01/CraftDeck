// Package network implements FR-21~25: an app-wide "외부 접속 허용" toggle
// backed by real UPnP(IGD)/NAT-PMP port-forwarding automation (FR-22),
// falling back to on-screen manual instructions when the router doesn't
// support either or automatic setup fails (FR-23), plus a registry of what
// CraftDeck itself has registered so the operator can review/revoke
// individual mappings (FR-24). One toggle covers both the management web
// UI port and every directly-reachable Minecraft game port (the singleton
// Velocity proxy's port, and any independently-exposed server's own port)
// -- per the operator's request, these aren't split into separate
// web/game switches (see internal/api's ReconcileGamePorts, which maps/
// unmaps a running instance's port automatically as it starts/stops).
package network

import (
	"context"
	"database/sql"
)

// Settings mirrors the `network_settings` singleton row (see
// internal/db/migrations/0002_network_settings.sql,
// 0003_merge_wan_toggle.sql).
type Settings struct {
	WANEnabled bool `json:"wan_enabled"`
}

type SettingsRepository struct {
	db *sql.DB
}

func NewSettingsRepository(db *sql.DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) Get(ctx context.Context) (*Settings, error) {
	var s Settings
	err := r.db.QueryRowContext(ctx,
		`SELECT wan_enabled FROM network_settings WHERE id = 1`,
	).Scan(&s.WANEnabled)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SettingsRepository) SetWANEnabled(ctx context.Context, enabled bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE network_settings SET wan_enabled = ? WHERE id = 1`, enabled)
	return err
}
