package network

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PortMapping mirrors one row of the `port_mappings` table (see
// internal/db/migrations/0001_init.sql -- the table predates this package
// but was never wired up until now). InstanceID is nil for the web UI
// port's mapping, which isn't tied to any one Minecraft instance; the
// game-port phase (FR-25's other half) will populate it per exposed
// instance.
type PortMapping struct {
	ID           string  `json:"id"`
	InstanceID   *string `json:"instance_id,omitempty"`
	ExternalPort int     `json:"external_port"`
	InternalPort int     `json:"internal_port"`
	Protocol     string  `json:"protocol"` // "tcp" or "udp"
	Method       string  `json:"method"`   // "upnp", "natpmp", or "manual"
	CreatedAt    string  `json:"created_at"`
}

type MappingRepository struct {
	db *sql.DB
}

func NewMappingRepository(db *sql.DB) *MappingRepository {
	return &MappingRepository{db: db}
}

func (r *MappingRepository) Create(ctx context.Context, m *PortMapping) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	m.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO port_mappings (id, instance_id, external_port, internal_port, protocol, method, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.InstanceID, m.ExternalPort, m.InternalPort, m.Protocol, m.Method, m.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert port mapping: %w", err)
	}
	return nil
}

func (r *MappingRepository) List(ctx context.Context) ([]*PortMapping, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, instance_id, external_port, internal_port, protocol, method, created_at
		FROM port_mappings ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list port mappings: %w", err)
	}
	defer rows.Close()

	out := []*PortMapping{}
	for rows.Next() {
		m, err := scanMapping(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *MappingRepository) Get(ctx context.Context, id string) (*PortMapping, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, instance_id, external_port, internal_port, protocol, method, created_at
		FROM port_mappings WHERE id = ?`, id)
	return scanMapping(row)
}

// GetWebMapping returns the (at most one) mapping not tied to any instance
// -- by convention, that's the web UI port's own mapping (see the
// InstanceID doc comment above).
func (r *MappingRepository) GetWebMapping(ctx context.Context) (*PortMapping, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, instance_id, external_port, internal_port, protocol, method, created_at
		FROM port_mappings WHERE instance_id IS NULL LIMIT 1`)
	return scanMapping(row)
}

// GetByInstance returns the (at most one) mapping registered for
// instanceID -- a running proxy or independently-exposed server's own
// game-port mapping (see internal/api's ReconcileGamePorts). Returns
// sql.ErrNoRows (via scanMapping) if that instance has no mapping right
// now, which is the normal state for a server sitting behind the proxy or
// one that isn't currently running.
func (r *MappingRepository) GetByInstance(ctx context.Context, instanceID string) (*PortMapping, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, instance_id, external_port, internal_port, protocol, method, created_at
		FROM port_mappings WHERE instance_id = ? LIMIT 1`, instanceID)
	return scanMapping(row)
}

func (r *MappingRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM port_mappings WHERE id = ?`, id)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMapping(row rowScanner) (*PortMapping, error) {
	var m PortMapping
	if err := row.Scan(&m.ID, &m.InstanceID, &m.ExternalPort, &m.InternalPort, &m.Protocol, &m.Method, &m.CreatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}
