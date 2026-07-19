// Package update implements the operator-facing update channel/check
// frequency settings (FR: stable/beta/canary apt channels, plus how often
// craftdeckd re-checks the apt repo for a newer version). See
// internal/api/handlers_system.go for how these settings gate the actual
// version check and drive /etc/apt/sources.list.d/craftdeck.list.
package update

import (
	"context"
	"database/sql"
	"time"
)

// Channel is one of the three apt repository components craftdeckd's own
// package is published into (see packaging/apt-repo/conf/distributions and
// .github/workflows/release.yml). Cumulative: stable ships into all three,
// beta into beta+canary, canary into canary only -- so switching to a
// "lower" channel never leaves an operator without at least the latest
// stable build available.
type Channel string

const (
	ChannelStable Channel = "stable"
	ChannelBeta   Channel = "beta"
	ChannelCanary Channel = "canary"
)

// aptComponent is the apt repository component name each channel maps to
// -- stable's is "main" for historical reasons (it predates the other two
// channels and existing installs' sources.list already say "main").
func (c Channel) aptComponent() string {
	if c == ChannelStable {
		return "main"
	}
	return string(c)
}

// AptComponent exposes aptComponent for callers outside this package (see
// handlers_system.go's channel-aware Packages-index URL, sources.go's
// sources.list line).
func (c Channel) AptComponent() string {
	return c.aptComponent()
}

// CheckFrequency controls how often handleCraftdeckVersion is allowed to
// actually hit the apt repo rather than reply from Settings' cached
// last-checked result.
type CheckFrequency string

const (
	CheckEveryVisit CheckFrequency = "every_visit"
	CheckDaily      CheckFrequency = "daily"
	CheckWeekly     CheckFrequency = "weekly"
	CheckMonthly    CheckFrequency = "monthly"
)

// Settings mirrors the `update_settings` singleton row (see
// internal/db/migrations/0011_update_settings.sql).
type Settings struct {
	Channel             Channel        `json:"channel"`
	CheckFrequency      CheckFrequency `json:"check_frequency"`
	CachedLatestVersion string         `json:"-"`
	LastCheckedAt       *time.Time     `json:"-"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Get(ctx context.Context) (*Settings, error) {
	var s Settings
	var channel, freq string
	var lastCheckedAt sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT channel, check_frequency, cached_latest_version, last_checked_at FROM update_settings WHERE id = 1`,
	).Scan(&channel, &freq, &s.CachedLatestVersion, &lastCheckedAt)
	if err != nil {
		return nil, err
	}
	s.Channel = Channel(channel)
	s.CheckFrequency = CheckFrequency(freq)
	s.LastCheckedAt = parseNullTime(lastCheckedAt)
	return &s, nil
}

func parseNullTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, ns.String)
	if err != nil {
		return nil
	}
	return &t
}

// SetChannelAndFrequency updates both fields together -- the PUT endpoint
// always sends both, and there's no scenario where one changes without the
// other being re-sent (see handlers_system.go's handleSetUpdateSettings).
func (r *Repository) SetChannelAndFrequency(ctx context.Context, channel Channel, freq CheckFrequency) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE update_settings SET channel = ?, check_frequency = ? WHERE id = 1`,
		string(channel), string(freq),
	)
	return err
}

// RecordCheck persists the outcome of an actual (non-cached) apt repo
// check, so the next handleCraftdeckVersion call within the configured
// check_frequency window can reply from this cache instead of re-fetching.
func (r *Repository) RecordCheck(ctx context.Context, latestVersion string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE update_settings SET cached_latest_version = ?, last_checked_at = ? WHERE id = 1`,
		latestVersion, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}
