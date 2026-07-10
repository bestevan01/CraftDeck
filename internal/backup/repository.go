package backup

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Backup mirrors the `backups` table (see internal/db/migrations/0001_init.sql).
type Backup struct {
	ID         string    `json:"id"`
	InstanceID string    `json:"instance_id"`
	Filename   string    `json:"filename"` // relative to dataDir/backups/<instance_id>/
	SizeBytes  int64     `json:"size_bytes"`
	CreatedAt  time.Time `json:"created_at"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, b *Backup) error {
	b.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO backups (id, instance_id, filename, size_bytes, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		b.ID, b.InstanceID, b.Filename, b.SizeBytes, b.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert backup: %w", err)
	}
	return nil
}

func (r *Repository) ListByInstance(ctx context.Context, instanceID string) ([]*Backup, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, instance_id, filename, size_bytes, created_at
		FROM backups WHERE instance_id = ? ORDER BY created_at DESC`, instanceID)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	defer rows.Close()

	out := []*Backup{} // never nil: frontend gets `[]`, not `null`, when empty
	for rows.Next() {
		b, err := scanBackup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (*Backup, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, instance_id, filename, size_bytes, created_at
		FROM backups WHERE id = ?`, id)
	return scanBackup(row)
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, id)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBackup(row rowScanner) (*Backup, error) {
	var b Backup
	var createdAt string
	err := row.Scan(&b.ID, &b.InstanceID, &b.Filename, &b.SizeBytes, &createdAt)
	if err != nil {
		return nil, err
	}
	b.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return &b, nil
}
