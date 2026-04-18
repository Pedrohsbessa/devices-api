package device

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Device is the aggregate root of this package. It models a uniquely
// identifiable unit (physical or virtual) tracked by the system.
//
// All fields are unexported and observed through methods. Mutations happen
// exclusively through domain behaviors defined on *Device so that invariants
// are always preserved. See the package doc for rationale.
type Device struct {
	id        uuid.UUID
	name      string
	brand     string
	state     State
	createdAt time.Time
}

// NewDevice creates a Device with a freshly generated UUIDv7 and the
// provided creation timestamp (injected for determinism and testability).
// New devices start in StateAvailable.
func NewDevice(name, brand string, now time.Time) (*Device, error) {
	name = strings.TrimSpace(name)
	brand = strings.TrimSpace(brand)
	if name == "" {
		return nil, ErrNameRequired
	}
	if brand == "" {
		return nil, ErrBrandRequired
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generating device id: %w", err)
	}
	return &Device{
		id:        id,
		name:      name,
		brand:     brand,
		state:     StateAvailable,
		createdAt: now.UTC(),
	}, nil
}

// Reconstruct rebuilds a Device from trusted persistence fields without
// re-running validations. Intended exclusively for the repository adapter
// loading rows from the database, where invariants are guaranteed by the
// table's CHECK constraints. Application code accepting external input
// must go through NewDevice instead.
func Reconstruct(id uuid.UUID, name, brand string, state State, createdAt time.Time) *Device {
	return &Device{
		id:        id,
		name:      name,
		brand:     brand,
		state:     state,
		createdAt: createdAt.UTC(),
	}
}

// ID returns the device's stable identifier.
func (d *Device) ID() uuid.UUID { return d.id }

// Name returns the device's human-readable name.
func (d *Device) Name() string { return d.name }

// Brand returns the device's brand.
func (d *Device) Brand() string { return d.brand }

// State returns the device's current lifecycle state.
func (d *Device) State() State { return d.state }

// CreatedAt returns the moment the device was created (UTC).
func (d *Device) CreatedAt() time.Time { return d.createdAt }

// Rename updates the device's name. Fails with ErrDeviceInUse when the
// device is currently in use, or with ErrNameRequired when the provided
// value is empty after trimming whitespace.
//
// The in-use check runs first: a locked resource cannot be updated
// regardless of the payload, so that precondition takes precedence over
// input validation.
func (d *Device) Rename(name string) error {
	if d.state == StateInUse {
		return ErrDeviceInUse
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNameRequired
	}
	d.name = name
	return nil
}

// ChangeBrand updates the device's brand. Fails with ErrDeviceInUse when
// the device is currently in use, or with ErrBrandRequired when the
// provided value is empty after trimming whitespace.
func (d *Device) ChangeBrand(brand string) error {
	if d.state == StateInUse {
		return ErrDeviceInUse
	}
	brand = strings.TrimSpace(brand)
	if brand == "" {
		return ErrBrandRequired
	}
	d.brand = brand
	return nil
}

// ChangeState transitions the device to the provided state. All
// transitions between the three canonical states are permitted: this is
// how a device is unblocked for name and brand edits after having been
// in use. Fails with ErrInvalidState when s is not a known state.
func (d *Device) ChangeState(s State) error {
	if !s.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidState, string(s))
	}
	d.state = s
	return nil
}

// CanDelete reports whether the device is deletable. Returns nil when
// deletion is permitted and ErrDeviceInUse when the device is in use.
// The service layer calls this before invoking the repository's Delete.
func (d *Device) CanDelete() error {
	if d.state == StateInUse {
		return ErrDeviceInUse
	}
	return nil
}
