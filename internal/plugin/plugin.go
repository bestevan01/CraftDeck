// Package plugin tracks installed plugins/mods (requirements.md FR-5~9): a
// database record per installed file plus the enable/disable convention
// used on disk (Paper/Bukkit-family servers simply skip any file in
// plugins/ that isn't named *.jar, so disabling one is just an extension
// rename).
package plugin

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const disabledSuffix = ".disabled"

// Plugin mirrors the `plugins` table (see internal/db/migrations/0001_init.sql).
type Plugin struct {
	ID                    string    `json:"id"`
	InstanceID            string    `json:"instance_id"`
	Source                string    `json:"source"` // "modrinth" or "upload"
	ModrinthProjectID     string    `json:"modrinth_project_id,omitempty"`
	ModrinthVersionID     string    `json:"modrinth_version_id,omitempty"`
	Filename              string    `json:"filename"` // on-disk name, without any .disabled suffix
	SHA512                string    `json:"sha512,omitempty"`
	Enabled               bool      `json:"enabled"`
	InstalledAsDependency bool      `json:"installed_as_dependency"`
	CreatedAt             time.Time `json:"created_at"`
}

// DiskFilename is the plugin's actual filename on disk right now --
// Filename plus ".disabled" when it's turned off.
func (p *Plugin) DiskFilename() string {
	if p.Enabled {
		return p.Filename
	}
	return p.Filename + disabledSuffix
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, p *Plugin) error {
	p.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO plugins (
			id, instance_id, source, modrinth_project_id, modrinth_version_id,
			filename, sha512, enabled, installed_as_dependency, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.InstanceID, p.Source, p.ModrinthProjectID, p.ModrinthVersionID,
		p.Filename, p.SHA512, p.Enabled, p.InstalledAsDependency,
		p.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert plugin: %w", err)
	}
	return nil
}

func (r *Repository) ListByInstance(ctx context.Context, instanceID string) ([]*Plugin, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, instance_id, source, modrinth_project_id, modrinth_version_id,
			filename, sha512, enabled, installed_as_dependency, created_at
		FROM plugins WHERE instance_id = ? ORDER BY created_at`, instanceID)
	if err != nil {
		return nil, fmt.Errorf("list plugins: %w", err)
	}
	defer rows.Close()

	out := []*Plugin{} // never nil: frontend gets `[]`, not `null`, when empty
	for rows.Next() {
		p, err := scanPlugin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// FindByModrinthProject looks up an already-installed plugin for instanceID
// by its Modrinth project ID, used to avoid re-installing a dependency
// that's already present (FR-6c).
func (r *Repository) FindByModrinthProject(ctx context.Context, instanceID, projectID string) (*Plugin, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, instance_id, source, modrinth_project_id, modrinth_version_id,
			filename, sha512, enabled, installed_as_dependency, created_at
		FROM plugins WHERE instance_id = ? AND modrinth_project_id = ?`, instanceID, projectID)
	p, err := scanPlugin(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (r *Repository) Get(ctx context.Context, id string) (*Plugin, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, instance_id, source, modrinth_project_id, modrinth_version_id,
			filename, sha512, enabled, installed_as_dependency, created_at
		FROM plugins WHERE id = ?`, id)
	return scanPlugin(row)
}

func (r *Repository) SetEnabled(ctx context.Context, id string, enabled bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE plugins SET enabled = ? WHERE id = ?`, enabled, id)
	return err
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM plugins WHERE id = ?`, id)
	return err
}

// DeleteByInstance removes every plugin/mod record for instanceID --
// called when the instance itself is deleted (see handleDeleteInstance),
// since `plugins.instance_id` has a foreign key on `instances(id)` with no
// ON DELETE CASCADE, so leftover rows would otherwise make the instance
// delete itself fail with a foreign key constraint error.
func (r *Repository) DeleteByInstance(ctx context.Context, instanceID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM plugins WHERE instance_id = ?`, instanceID)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPlugin(row rowScanner) (*Plugin, error) {
	var p Plugin
	var createdAt string
	var modrinthProjectID, modrinthVersionID, sha512 sql.NullString
	err := row.Scan(
		&p.ID, &p.InstanceID, &p.Source, &modrinthProjectID, &modrinthVersionID,
		&p.Filename, &sha512, &p.Enabled, &p.InstalledAsDependency, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	p.ModrinthProjectID = modrinthProjectID.String
	p.ModrinthVersionID = modrinthVersionID.String
	p.SHA512 = sha512.String
	p.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return &p, nil
}
