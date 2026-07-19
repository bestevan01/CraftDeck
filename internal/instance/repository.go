package instance

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, inst *Instance) error {
	inst.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO instances (
			id, name, kind, loader, loader_version, mc_version, java_major,
			game_port, rcon_port, rcon_password, cpu_quota_percent,
			memory_max_mb, work_dir, status, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		inst.ID, inst.Name, inst.Kind, inst.Loader, inst.LoaderVersion, inst.MCVersion,
		inst.JavaMajor, inst.GamePort, inst.RCONPort, inst.RCONPassword,
		inst.CPUQuotaPercent, inst.MemoryMaxMB, inst.WorkDir, inst.Status,
		inst.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert instance: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id string) (*Instance, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, kind, loader, loader_version, mc_version, java_major,
			game_port, rcon_port, rcon_password, cpu_quota_percent,
			memory_max_mb, work_dir, status, created_at, proxy_opt_out
		FROM instances WHERE id = ?`, id)
	return scanInstance(row)
}

func (r *Repository) List(ctx context.Context) ([]*Instance, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, kind, loader, loader_version, mc_version, java_major,
			game_port, rcon_port, rcon_password, cpu_quota_percent,
			memory_max_mb, work_dir, status, created_at, proxy_opt_out
		FROM instances ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer rows.Close()

	out := []*Instance{} // never nil: frontend gets `[]`, not `null`, when empty
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inst)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateStatus(ctx context.Context, id string, status Status) error {
	_, err := r.db.ExecContext(ctx, `UPDATE instances SET status = ? WHERE id = ?`, status, id)
	return err
}

// UpdateSettings changes the port/resource-allocation fields an operator can
// edit while an instance is stopped (FR-12: port and memory/CPU allocation).
func (r *Repository) UpdateSettings(ctx context.Context, id string, gamePort, rconPort, cpuQuotaPercent, memoryMaxMB int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE instances
		SET game_port = ?, rcon_port = ?, cpu_quota_percent = ?, memory_max_mb = ?
		WHERE id = ?`, gamePort, rconPort, cpuQuotaPercent, memoryMaxMB, id)
	return err
}

// UpdateVersion records a new mc_version (and the Java major it needs to
// run under -- see loader.FetchVelocityJavaMinimum, since a Velocity major
// upgrade can bump this) after an instance's jar has been replaced in
// place (currently only used to upgrade the singleton Velocity proxy --
// see handleUpgradeProxy -- since every other instance's version is fixed
// at creation and changed by recreating it instead).
func (r *Repository) UpdateVersion(ctx context.Context, id, mcVersion string, javaMajor int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE instances SET mc_version = ?, java_major = ? WHERE id = ?`, mcVersion, javaMajor, id)
	return err
}

// UpdateLoaderVersion records which build (BuildLister.BuildInfo.ID) is
// currently installed after a pinned reinstall (FR-4's build-selection
// extension) -- empty string means "always latest", matching a freshly
// created instance's default.
func (r *Repository) UpdateLoaderVersion(ctx context.Context, id, loaderVersion string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE instances SET loader_version = ? WHERE id = ?`, loaderVersion, id)
	return err
}

// SetProxyOptOut records whether an operator has explicitly converted this
// server to independent exposure (true) or (re-)registered it behind the
// proxy (false) -- see the Instance.ProxyOptOut doc comment.
func (r *Repository) SetProxyOptOut(ctx context.Context, id string, optOut bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE instances SET proxy_opt_out = ? WHERE id = ?`, optOut, id)
	return err
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM instances WHERE id = ?`, id)
	return err
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanInstance(row rowScanner) (*Instance, error) {
	var inst Instance
	var createdAt string
	err := row.Scan(
		&inst.ID, &inst.Name, &inst.Kind, &inst.Loader, &inst.LoaderVersion,
		&inst.MCVersion, &inst.JavaMajor, &inst.GamePort, &inst.RCONPort,
		&inst.RCONPassword, &inst.CPUQuotaPercent, &inst.MemoryMaxMB,
		&inst.WorkDir, &inst.Status, &createdAt, &inst.ProxyOptOut,
	)
	if err != nil {
		return nil, err
	}
	inst.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return &inst, nil
}
