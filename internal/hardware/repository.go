// Package hardware detects whether a Raspberry Pi 5's official Active
// Cooler is physically attached, and -- only once that's confirmed --
// lets the operator apply and self-validate an overclock. See
// DetectActiveCooler's doc comment for why presence can't just be read off
// a sysfs node, and Overclock/RunBenchmark for how the setting is applied
// and verified.
package hardware

import (
	"context"
	"database/sql"
	"time"
)

// Config mirrors the `hardware_settings` singleton row (see
// internal/db/migrations/0009_hardware.sql).
type Config struct {
	CoolerDetected  bool       `json:"cooler_detected"`
	CoolerCheckedAt *time.Time `json:"cooler_checked_at,omitempty"`

	OverclockEnabled          bool       `json:"overclock_enabled"`
	OverclockPreset           string     `json:"overclock_preset"`
	OverclockArmFreq          int        `json:"overclock_arm_freq,omitempty"`
	OverclockOverVoltageDelta int        `json:"overclock_over_voltage_delta,omitempty"`
	OverclockAppliedAt        *time.Time `json:"overclock_applied_at,omitempty"`

	LastBenchmarkResult string     `json:"last_benchmark_result"`
	LastBenchmarkAt     *time.Time `json:"last_benchmark_at,omitempty"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Get(ctx context.Context) (*Config, error) {
	var c Config
	var coolerCheckedAt, overclockAppliedAt, lastBenchmarkAt sql.NullString
	var armFreq, overVoltageDelta sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT cooler_detected, cooler_checked_at, overclock_enabled, overclock_preset,
			overclock_arm_freq, overclock_over_voltage_delta, overclock_applied_at,
			last_benchmark_result, last_benchmark_at
		FROM hardware_settings WHERE id = 1`,
	).Scan(&c.CoolerDetected, &coolerCheckedAt, &c.OverclockEnabled, &c.OverclockPreset,
		&armFreq, &overVoltageDelta, &overclockAppliedAt,
		&c.LastBenchmarkResult, &lastBenchmarkAt)
	if err != nil {
		return nil, err
	}
	c.CoolerCheckedAt = parseNullTime(coolerCheckedAt)
	c.OverclockAppliedAt = parseNullTime(overclockAppliedAt)
	c.LastBenchmarkAt = parseNullTime(lastBenchmarkAt)
	if armFreq.Valid {
		c.OverclockArmFreq = int(armFreq.Int64)
	}
	if overVoltageDelta.Valid {
		c.OverclockOverVoltageDelta = int(overVoltageDelta.Int64)
	}
	return &c, nil
}

// SetCoolerDetected records the one-shot detection result (see
// DetectActiveCooler) and marks it as having run, via cooler_checked_at,
// so cmd/craftdeckd/main.go's startup hook never repeats it.
func (r *Repository) SetCoolerDetected(ctx context.Context, detected bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE hardware_settings SET cooler_detected = ?, cooler_checked_at = ? WHERE id = 1`,
		detected, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ClearCoolerDetection resets cooler_checked_at to NULL so the next
// craftdeckd startup re-runs DetectActiveCooler -- used by the manual
// "다시 감지" action for an operator who added a cooler after the
// automatic one-shot check already ran (and found none).
func (r *Repository) ClearCoolerDetection(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE hardware_settings SET cooler_detected = 0, cooler_checked_at = NULL WHERE id = 1`)
	return err
}

// SetOverclock persists the operator's chosen overclock values after
// Overclock.Apply has successfully written them to config.txt. A reboot
// (triggered separately) is what actually makes them take effect.
func (r *Repository) SetOverclock(ctx context.Context, enabled bool, preset string, armFreq, overVoltageDeltaUV int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE hardware_settings
		SET overclock_enabled = ?, overclock_preset = ?, overclock_arm_freq = ?,
			overclock_over_voltage_delta = ?, overclock_applied_at = ?
		WHERE id = 1`,
		enabled, preset, armFreq, overVoltageDeltaUV, time.Now().UTC().Format(time.RFC3339))
	return err
}

// SetBenchmarkResult records the outcome of the most recent stability
// self-test (see RunBenchmark) -- "pass" or "fail", surfaced next to the
// overclock controls in the UI.
func (r *Repository) SetBenchmarkResult(ctx context.Context, result string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE hardware_settings SET last_benchmark_result = ?, last_benchmark_at = ? WHERE id = 1`,
		result, time.Now().UTC().Format(time.RFC3339))
	return err
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
