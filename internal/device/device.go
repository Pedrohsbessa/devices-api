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
