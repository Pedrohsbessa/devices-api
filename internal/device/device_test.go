package device_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Pedrohsbessa/devices-api/internal/device"
)

// fixedNow is a deterministic timestamp used across tests so we can assert
// equality instead of tolerance.
var fixedNow = time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)

func newAvailableDevice(t *testing.T) *device.Device {
	t.Helper()
	d, err := device.NewDevice("laptop", "lenovo", fixedNow)
	require.NoError(t, err)
	return d
}

func newInState(t *testing.T, s device.State) *device.Device {
	t.Helper()
	d := newAvailableDevice(t)
	if s != device.StateAvailable {
		require.NoError(t, d.ChangeState(s))
	}
	return d
}

func TestNewDevice_HappyPath(t *testing.T) {
	d, err := device.NewDevice("  laptop  ", "  lenovo  ", fixedNow)
	require.NoError(t, err)
	require.Equal(t, "laptop", d.Name(), "name should be trimmed")
	require.Equal(t, "lenovo", d.Brand(), "brand should be trimmed")
	require.Equal(t, device.StateAvailable, d.State())
	require.True(t, d.CreatedAt().Equal(fixedNow))
	require.NotEqual(t, uuid.Nil, d.ID())
}

func TestNewDevice_NormalizesCreatedAtToUTC(t *testing.T) {
	brt := time.FixedZone("BRT", -3*3600)
	localNow := time.Date(2026, 4, 18, 9, 0, 0, 0, brt)

	d, err := device.NewDevice("laptop", "lenovo", localNow)
	require.NoError(t, err)
	require.Equal(t, time.UTC, d.CreatedAt().Location())
	require.True(t, d.CreatedAt().Equal(localNow))
}

func TestNewDevice_Validation(t *testing.T) {
	tests := []struct {
		name       string
		inputName  string
		inputBrand string
		wantErr    error
	}{
		{"empty name", "", "lenovo", device.ErrNameRequired},
		{"whitespace-only name", "   ", "lenovo", device.ErrNameRequired},
		{"empty brand", "laptop", "", device.ErrBrandRequired},
		{"whitespace-only brand", "laptop", "\t\n ", device.ErrBrandRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := device.NewDevice(tt.inputName, tt.inputBrand, fixedNow)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestNewDevice_GeneratesUniqueIDs(t *testing.T) {
	const n = 100
	seen := make(map[uuid.UUID]struct{}, n)
	for i := 0; i < n; i++ {
		d, err := device.NewDevice("x", "y", fixedNow)
		require.NoError(t, err)
		_, exists := seen[d.ID()]
		require.False(t, exists, "duplicate id at iteration %d", i)
		seen[d.ID()] = struct{}{}
	}
}

func TestReconstruct(t *testing.T) {
	id := uuid.New()
	createdAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	d := device.Reconstruct(id, "tablet", "apple", device.StateInUse, createdAt)

	require.Equal(t, id, d.ID())
	require.Equal(t, "tablet", d.Name())
	require.Equal(t, "apple", d.Brand())
	require.Equal(t, device.StateInUse, d.State())
	require.True(t, d.CreatedAt().Equal(createdAt))
}

func TestReconstruct_NormalizesCreatedAtToUTC(t *testing.T) {
	brt := time.FixedZone("BRT", -3*3600)
	local := time.Date(2026, 4, 18, 9, 0, 0, 0, brt)

	d := device.Reconstruct(uuid.New(), "x", "y", device.StateAvailable, local)

	require.Equal(t, time.UTC, d.CreatedAt().Location())
	require.True(t, d.CreatedAt().Equal(local))
}

func TestDevice_Rename(t *testing.T) {
	tests := []struct {
		name     string
		initial  device.State
		newName  string
		wantErr  error
		wantName string // asserted only when wantErr is nil
	}{
		{"available and valid", device.StateAvailable, "pro", nil, "pro"},
		{"inactive and valid", device.StateInactive, "pro", nil, "pro"},
		{"trims whitespace", device.StateAvailable, "  pro  ", nil, "pro"},
		{"in-use is blocked", device.StateInUse, "pro", device.ErrDeviceInUse, ""},
		{"empty name rejected", device.StateAvailable, "", device.ErrNameRequired, ""},
		{"whitespace-only name rejected", device.StateAvailable, "   ", device.ErrNameRequired, ""},
		{"in-use precedes empty-name check", device.StateInUse, "", device.ErrDeviceInUse, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newInState(t, tt.initial)
			before := d.Name()

			err := d.Rename(tt.newName)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Equal(t, before, d.Name(), "name must not change on error")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantName, d.Name())
		})
	}
}

func TestDevice_ChangeBrand(t *testing.T) {
	tests := []struct {
		name      string
		initial   device.State
		newBrand  string
		wantErr   error
		wantBrand string
	}{
		{"available and valid", device.StateAvailable, "dell", nil, "dell"},
		{"inactive and valid", device.StateInactive, "dell", nil, "dell"},
		{"trims whitespace", device.StateAvailable, "  dell  ", nil, "dell"},
		{"in-use is blocked", device.StateInUse, "dell", device.ErrDeviceInUse, ""},
		{"empty brand rejected", device.StateAvailable, "", device.ErrBrandRequired, ""},
		{"whitespace-only brand rejected", device.StateAvailable, "\t ", device.ErrBrandRequired, ""},
		{"in-use precedes empty-brand check", device.StateInUse, "", device.ErrDeviceInUse, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newInState(t, tt.initial)
			before := d.Brand()

			err := d.ChangeBrand(tt.newBrand)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Equal(t, before, d.Brand(), "brand must not change on error")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantBrand, d.Brand())
		})
	}
}

func TestDevice_ChangeState(t *testing.T) {
	tests := []struct {
		name    string
		initial device.State
		target  device.State
		wantErr error
	}{
		{"available to in-use", device.StateAvailable, device.StateInUse, nil},
		{"in-use to inactive", device.StateInUse, device.StateInactive, nil},
		{"inactive to available", device.StateInactive, device.StateAvailable, nil},
		{"same-state is a valid no-op transition", device.StateAvailable, device.StateAvailable, nil},
		{"unknown target rejected", device.StateAvailable, device.State("broken"), device.ErrInvalidState},
		{"empty target rejected", device.StateAvailable, device.State(""), device.ErrInvalidState},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newInState(t, tt.initial)

			err := d.ChangeState(tt.target)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Equal(t, tt.initial, d.State(), "state must not change on error")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.target, d.State())
		})
	}
}

func TestDevice_CanDelete(t *testing.T) {
	tests := []struct {
		name    string
		state   device.State
		wantErr error
	}{
		{"available is deletable", device.StateAvailable, nil},
		{"inactive is deletable", device.StateInactive, nil},
		{"in-use is blocked", device.StateInUse, device.ErrDeviceInUse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newInState(t, tt.state)

			err := d.CanDelete()

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestDevice_CreationTimeImmutability is the behavioral counterpart of the
// structural invariant: createdAt cannot be written from outside the package
// (enforced at compile time) and no mutator touches it (checked here).
func TestDevice_CreationTimeImmutability(t *testing.T) {
	d := newAvailableDevice(t)
	original := d.CreatedAt()

	require.NoError(t, d.Rename("new-name"))
	require.True(t, d.CreatedAt().Equal(original))

	require.NoError(t, d.ChangeBrand("new-brand"))
	require.True(t, d.CreatedAt().Equal(original))

	require.NoError(t, d.ChangeState(device.StateInUse))
	require.True(t, d.CreatedAt().Equal(original))

	require.NoError(t, d.ChangeState(device.StateInactive))
	require.True(t, d.CreatedAt().Equal(original))
}
