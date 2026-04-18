// Package postgres is the PostgreSQL adapter implementing device.Repository
// via the jackc/pgx/v5 driver.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Pedrohsbessa/devices-api/internal/device"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// Repository persists Device aggregates in PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository wires a repository around an existing connection pool.
// The caller owns the pool and is responsible for its lifecycle.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Compile-time assertion that *Repository satisfies device.Repository.
var _ device.Repository = (*Repository)(nil)

const createQuery = `
INSERT INTO devices (id, name, brand, state, created_at)
VALUES ($1, $2, $3, $4, $5)
`

// Create inserts a new device row. Duplicate-id collisions are not an
// expected path with application-generated UUIDv7s; any error here is
// returned wrapped for the caller to surface.
func (r *Repository) Create(ctx context.Context, d *device.Device) error {
	_, err := r.pool.Exec(ctx, createQuery,
		d.ID(), d.Name(), d.Brand(), string(d.State()), d.CreatedAt())
	if err != nil {
		return fmt.Errorf("postgres: create device: %w", err)
	}
	return nil
}

const getByIDQuery = `
SELECT id, name, brand, state, created_at
FROM devices
WHERE id = $1
`

// GetByID loads a single device, translating pgx.ErrNoRows to the domain
// sentinel device.ErrDeviceNotFound.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*device.Device, error) {
	var (
		dbID      uuid.UUID
		name      string
		brand     string
		state     string
		createdAt time.Time
	)
	err := r.pool.QueryRow(ctx, getByIDQuery, id).
		Scan(&dbID, &name, &brand, &state, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, device.ErrDeviceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get device: %w", err)
	}
	return device.Reconstruct(dbID, name, brand, device.State(state), createdAt), nil
}

// List builds a parameterised query from the filter. Ordering by id DESC
// leverages UUIDv7's monotonicity to mean "most recently created first"
// without requiring a separate index on created_at.
func (r *Repository) List(ctx context.Context, f device.ListFilter) ([]*device.Device, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	limit = min(limit, maxListLimit)
	offset := max(f.Offset, 0)

	args := make([]any, 0, 4)
	predicates := make([]string, 0, 2)
	if f.Brand != nil {
		args = append(args, *f.Brand)
		predicates = append(predicates, fmt.Sprintf("brand = $%d", len(args)))
	}
	if f.State != nil {
		args = append(args, string(*f.State))
		predicates = append(predicates, fmt.Sprintf("state = $%d", len(args)))
	}
	where := ""
	if len(predicates) > 0 {
		where = "WHERE " + strings.Join(predicates, " AND ")
	}
	args = append(args, limit, offset)
	query := fmt.Sprintf(`
SELECT id, name, brand, state, created_at
FROM devices
%s
ORDER BY id DESC
LIMIT $%d OFFSET $%d
`, where, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list devices: %w", err)
	}
	defer rows.Close()

	devices := make([]*device.Device, 0)
	for rows.Next() {
		var (
			id        uuid.UUID
			name      string
			brand     string
			state     string
			createdAt time.Time
		)
		if err := rows.Scan(&id, &name, &brand, &state, &createdAt); err != nil {
			return nil, fmt.Errorf("postgres: scan device: %w", err)
		}
		devices = append(devices, device.Reconstruct(id, name, brand, device.State(state), createdAt))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate devices: %w", err)
	}
	return devices, nil
}

const updateQuery = `
UPDATE devices
SET name = $1, brand = $2, state = $3
WHERE id = $4
`

// Update persists the mutable fields for an existing device. The
// repository does not enforce domain rules — the service mutates the
// aggregate in memory (where invariants are checked) and then calls
// Update to flush the new state.
func (r *Repository) Update(ctx context.Context, d *device.Device) error {
	tag, err := r.pool.Exec(ctx, updateQuery,
		d.Name(), d.Brand(), string(d.State()), d.ID())
	if err != nil {
		return fmt.Errorf("postgres: update device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return device.ErrDeviceNotFound
	}
	return nil
}

const deleteQuery = `DELETE FROM devices WHERE id = $1`

// Delete removes the row with the given id. Returns ErrDeviceNotFound
// when no row matched. The "in-use cannot be deleted" rule is enforced by
// the service layer via Device.CanDelete before this call.
func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, deleteQuery, id)
	if err != nil {
		return fmt.Errorf("postgres: delete device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return device.ErrDeviceNotFound
	}
	return nil
}
