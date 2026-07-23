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
	Title                 string    `json:"title,omitempty"` // Modrinth display name, e.g. "Sodium"; empty for uploads
	SHA512                string    `json:"sha512,omitempty"`
	Enabled               bool      `json:"enabled"`
	InstalledAsDependency bool      `json:"installed_as_dependency"`
	ParentPluginID        string    `json:"parent_plugin_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	// DependentOf lists every plugin ID that requires this one, per the
	// plugin_dependencies table (0014_plugin_dependencies.sql) -- not a DB
	// column, populated by handleListPlugins after the fact via
	// ListDependencyEdges. ParentPluginID only ever records the first
	// plugin that triggered this one's install; a shared dependency (Fabric
	// API being needed by half a dozen mods is the common case) needs all
	// of them, not just the first.
	DependentOf []string `json:"dependent_of,omitempty"`
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
	var parentPluginID sql.NullString
	if p.ParentPluginID != "" {
		parentPluginID = sql.NullString{String: p.ParentPluginID, Valid: true}
	}
	var title sql.NullString
	if p.Title != "" {
		title = sql.NullString{String: p.Title, Valid: true}
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO plugins (
			id, instance_id, source, modrinth_project_id, modrinth_version_id,
			filename, title, sha512, enabled, installed_as_dependency, parent_plugin_id, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.InstanceID, p.Source, p.ModrinthProjectID, p.ModrinthVersionID,
		p.Filename, title, p.SHA512, p.Enabled, p.InstalledAsDependency, parentPluginID,
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
			filename, title, sha512, enabled, installed_as_dependency, parent_plugin_id, created_at
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
			filename, title, sha512, enabled, installed_as_dependency, parent_plugin_id, created_at
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
			filename, title, sha512, enabled, installed_as_dependency, parent_plugin_id, created_at
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

// AddDependency records that parentID requires dependencyID, in addition to
// whatever ParentPluginID already says (which only ever holds the first
// plugin that triggered dependencyID's install). Idempotent -- installing
// the same mod twice, or two mods that both depend on the same shared
// dependency, must not fail or duplicate the edge.
func (r *Repository) AddDependency(ctx context.Context, parentID, dependencyID string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO plugin_dependencies (parent_plugin_id, dependency_plugin_id)
		VALUES (?, ?)`, parentID, dependencyID)
	return err
}

// DependencyEdge is one row of plugin_dependencies.
type DependencyEdge struct {
	ParentPluginID     string
	DependencyPluginID string
}

// ListDependencyEdges returns every (parent, dependency) edge among
// instanceID's plugins -- joined through plugins to scope by instance,
// since plugin_dependencies itself has no instance_id column (both sides of
// an edge always belong to the same instance, so joining on either works).
func (r *Repository) ListDependencyEdges(ctx context.Context, instanceID string) ([]DependencyEdge, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT pd.parent_plugin_id, pd.dependency_plugin_id
		FROM plugin_dependencies pd
		JOIN plugins p ON p.id = pd.dependency_plugin_id
		WHERE p.instance_id = ?`, instanceID)
	if err != nil {
		return nil, fmt.Errorf("list plugin dependency edges: %w", err)
	}
	defer rows.Close()

	var out []DependencyEdge
	for rows.Next() {
		var e DependencyEdge
		if err := rows.Scan(&e.ParentPluginID, &e.DependencyPluginID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
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
	var modrinthProjectID, modrinthVersionID, title, sha512, parentPluginID sql.NullString
	err := row.Scan(
		&p.ID, &p.InstanceID, &p.Source, &modrinthProjectID, &modrinthVersionID,
		&p.Filename, &title, &sha512, &p.Enabled, &p.InstalledAsDependency, &parentPluginID, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	p.ModrinthProjectID = modrinthProjectID.String
	p.ModrinthVersionID = modrinthVersionID.String
	p.Title = title.String
	p.SHA512 = sha512.String
	p.ParentPluginID = parentPluginID.String
	p.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return &p, nil
}
