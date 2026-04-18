package device

import (
	"context"

	"github.com/google/uuid"
)

// Repository is the persistence contract for Device aggregates. The service
// layer depends on this interface; concrete implementations live in
// dedicated adapter packages (e.g. internal/device/postgres).
type Repository interface {
	// Create persists a new device.
	Create(ctx context.Context, d *Device) error

	// GetByID returns the device with the given id, or ErrDeviceNotFound.
	GetByID(ctx context.Context, id uuid.UUID) (*Device, error)

	// List returns devices matching the filter, ordered from most recent
	// to oldest. Pagination is controlled by ListFilter.
	List(ctx context.Context, filter ListFilter) ([]*Device, error)

	// Update persists the mutable fields (name, brand, state) for an
	// existing device. Returns ErrDeviceNotFound if the id is unknown.
	Update(ctx context.Context, d *Device) error

	// Delete removes the device with the given id. Returns
	// ErrDeviceNotFound if the id is unknown. Domain rules (e.g. in-use
	// devices cannot be deleted) are enforced by the service before this
	// call.
	Delete(ctx context.Context, id uuid.UUID) error
}

// ListFilter describes the optional filters applied to Repository.List. A
// nil Brand or State pointer means "no filter on that dimension"; Limit
// values outside the implementation's sane range are clamped to defaults.
type ListFilter struct {
	Brand  *string
	State  *State
	Limit  int
	Offset int
}
