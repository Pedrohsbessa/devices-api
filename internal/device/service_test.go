package device_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Pedrohsbessa/devices-api/internal/device"
)

// ---------------------------------------------------------------------------
// Handwritten Repository stub
// ---------------------------------------------------------------------------

// repoStub implements device.Repository by delegating each method to a
// per-test function. A nil function causes a nil-pointer panic at the call
// site — that's by design: tests set only the hooks they need and a missed
// expectation surfaces as a clear stack trace.
type repoStub struct {
	CreateFunc  func(ctx context.Context, d *device.Device) error
	GetByIDFunc func(ctx context.Context, id uuid.UUID) (*device.Device, error)
	ListFunc    func(ctx context.Context, f device.ListFilter) ([]*device.Device, error)
	UpdateFunc  func(ctx context.Context, d *device.Device) error
	DeleteFunc  func(ctx context.Context, id uuid.UUID) error
}

func (r *repoStub) Create(ctx context.Context, d *device.Device) error {
	return r.CreateFunc(ctx, d)
}

func (r *repoStub) GetByID(ctx context.Context, id uuid.UUID) (*device.Device, error) {
	return r.GetByIDFunc(ctx, id)
}

func (r *repoStub) List(ctx context.Context, f device.ListFilter) ([]*device.Device, error) {
	return r.ListFunc(ctx, f)
}

func (r *repoStub) Update(ctx context.Context, d *device.Device) error {
	return r.UpdateFunc(ctx, d)
}

func (r *repoStub) Delete(ctx context.Context, id uuid.UUID) error {
	return r.DeleteFunc(ctx, id)
}

// Compile-time assertion that the stub still satisfies the real interface.
var _ device.Repository = (*repoStub)(nil)

func newService(repo *repoStub) *device.Service {
	return device.NewService(repo, func() time.Time { return fixedNow })
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestService_Create_HappyPath(t *testing.T) {
	var captured *device.Device
	repo := &repoStub{
		CreateFunc: func(_ context.Context, d *device.Device) error {
			captured = d
			return nil
		},
	}
	svc := newService(repo)

	got, err := svc.Create(context.Background(), "laptop", "lenovo")

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "laptop", got.Name())
	require.Equal(t, "lenovo", got.Brand())
	require.Equal(t, device.StateAvailable, got.State())
	require.True(t, got.CreatedAt().Equal(fixedNow))
	require.NotEqual(t, uuid.Nil, got.ID())
	require.Same(t, got, captured, "service must persist the exact aggregate it constructed")
}

func TestService_Create_ValidationErrorsSkipRepository(t *testing.T) {
	tests := []struct {
		name, inputName, inputBrand string
		wantErr                     error
	}{
		{"empty name", "", "lenovo", device.ErrNameRequired},
		{"empty brand", "laptop", "", device.ErrBrandRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &repoStub{
				CreateFunc: func(context.Context, *device.Device) error {
					t.Fatalf("Repository.Create must not be called when domain validation fails")
					return nil
				},
			}
			svc := newService(repo)

			_, err := svc.Create(context.Background(), tt.inputName, tt.inputBrand)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestService_Create_PropagatesRepositoryError(t *testing.T) {
	sentinel := errors.New("boom")
	repo := &repoStub{
		CreateFunc: func(context.Context, *device.Device) error { return sentinel },
	}
	svc := newService(repo)

	_, err := svc.Create(context.Background(), "laptop", "lenovo")
	require.ErrorIs(t, err, sentinel)
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

func TestService_Get_ReturnsWhatRepoReturns(t *testing.T) {
	want := newAvailableDevice(t)
	var askedID uuid.UUID
	repo := &repoStub{
		GetByIDFunc: func(_ context.Context, id uuid.UUID) (*device.Device, error) {
			askedID = id
			return want, nil
		},
	}
	svc := newService(repo)

	got, err := svc.Get(context.Background(), want.ID())
	require.NoError(t, err)
	require.Same(t, want, got)
	require.Equal(t, want.ID(), askedID)
}

func TestService_Get_NotFound(t *testing.T) {
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) {
			return nil, device.ErrDeviceNotFound
		},
	}
	svc := newService(repo)

	_, err := svc.Get(context.Background(), uuid.New())
	require.ErrorIs(t, err, device.ErrDeviceNotFound)
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestService_List_ForwardsFilter(t *testing.T) {
	brand := "lenovo"
	state := device.StateInUse
	want := device.ListFilter{Brand: &brand, State: &state, Limit: 10, Offset: 5}

	var got device.ListFilter
	expected := []*device.Device{newAvailableDevice(t)}
	repo := &repoStub{
		ListFunc: func(_ context.Context, f device.ListFilter) ([]*device.Device, error) {
			got = f
			return expected, nil
		},
	}
	svc := newService(repo)

	out, err := svc.List(context.Background(), want)
	require.NoError(t, err)
	require.Equal(t, expected, out)
	require.Equal(t, want, got, "filter must be forwarded unchanged")
}

func TestService_List_PropagatesError(t *testing.T) {
	sentinel := errors.New("db down")
	repo := &repoStub{
		ListFunc: func(context.Context, device.ListFilter) ([]*device.Device, error) {
			return nil, sentinel
		},
	}
	svc := newService(repo)

	_, err := svc.List(context.Background(), device.ListFilter{})
	require.ErrorIs(t, err, sentinel)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestService_Update_OnlyName_AppliesAndPersists(t *testing.T) {
	d := newAvailableDevice(t)
	var persisted *device.Device
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		UpdateFunc: func(_ context.Context, d *device.Device) error {
			persisted = d
			return nil
		},
	}
	svc := newService(repo)

	newName := "pro"
	got, err := svc.Update(context.Background(), d.ID(), device.UpdatePatch{Name: &newName})

	require.NoError(t, err)
	require.Equal(t, "pro", got.Name())
	require.Same(t, d, persisted, "service must persist the same aggregate it read")
	require.Equal(t, "pro", persisted.Name())
}

func TestService_Update_AllFields_Applied(t *testing.T) {
	d := newAvailableDevice(t)
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		UpdateFunc:  func(context.Context, *device.Device) error { return nil },
	}
	svc := newService(repo)

	newName := "pro"
	newBrand := "dell"
	newState := device.StateInactive
	got, err := svc.Update(context.Background(), d.ID(), device.UpdatePatch{
		Name:  &newName,
		Brand: &newBrand,
		State: &newState,
	})

	require.NoError(t, err)
	require.Equal(t, "pro", got.Name())
	require.Equal(t, "dell", got.Brand())
	require.Equal(t, device.StateInactive, got.State())
}

// TestService_Update_StateAppliedBeforeRename locks the ordering: if the
// service applied Rename before ChangeState, this test would fail with
// ErrDeviceInUse instead of succeeding.
func TestService_Update_StateAppliedBeforeRename(t *testing.T) {
	d := newInState(t, device.StateInUse)
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		UpdateFunc:  func(context.Context, *device.Device) error { return nil },
	}
	svc := newService(repo)

	unlocked := device.StateAvailable
	newName := "unlocked"
	got, err := svc.Update(context.Background(), d.ID(), device.UpdatePatch{
		State: &unlocked,
		Name:  &newName,
	})

	require.NoError(t, err)
	require.Equal(t, device.StateAvailable, got.State())
	require.Equal(t, "unlocked", got.Name())
}

func TestService_Update_InUseRejectsRenameWithoutStateChange(t *testing.T) {
	d := newInState(t, device.StateInUse)
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		UpdateFunc: func(context.Context, *device.Device) error {
			t.Fatalf("Repository.Update must not be called when the domain blocks the change")
			return nil
		},
	}
	svc := newService(repo)

	newName := "nope"
	_, err := svc.Update(context.Background(), d.ID(), device.UpdatePatch{Name: &newName})
	require.ErrorIs(t, err, device.ErrDeviceInUse)
}

func TestService_Update_InvalidStateRejected(t *testing.T) {
	d := newAvailableDevice(t)
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		UpdateFunc: func(context.Context, *device.Device) error {
			t.Fatalf("Repository.Update must not be called for invalid state transitions")
			return nil
		},
	}
	svc := newService(repo)

	bad := device.State("broken")
	_, err := svc.Update(context.Background(), d.ID(), device.UpdatePatch{State: &bad})
	require.ErrorIs(t, err, device.ErrInvalidState)
}

func TestService_Update_EmptyPatch_StillPersists(t *testing.T) {
	d := newAvailableDevice(t)
	updateCalled := false
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		UpdateFunc: func(context.Context, *device.Device) error {
			updateCalled = true
			return nil
		},
	}
	svc := newService(repo)

	got, err := svc.Update(context.Background(), d.ID(), device.UpdatePatch{})
	require.NoError(t, err)
	require.True(t, updateCalled, "empty patch still triggers a no-op persist")
	require.Same(t, d, got)
}

func TestService_Update_NotFoundSkipsPersist(t *testing.T) {
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) {
			return nil, device.ErrDeviceNotFound
		},
		UpdateFunc: func(context.Context, *device.Device) error {
			t.Fatalf("Repository.Update must not be called when the device is not found")
			return nil
		},
	}
	svc := newService(repo)

	newName := "x"
	_, err := svc.Update(context.Background(), uuid.New(), device.UpdatePatch{Name: &newName})
	require.ErrorIs(t, err, device.ErrDeviceNotFound)
}

func TestService_Update_PropagatesRepoUpdateError(t *testing.T) {
	d := newAvailableDevice(t)
	sentinel := errors.New("disk full")
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		UpdateFunc:  func(context.Context, *device.Device) error { return sentinel },
	}
	svc := newService(repo)

	newName := "x"
	_, err := svc.Update(context.Background(), d.ID(), device.UpdatePatch{Name: &newName})
	require.ErrorIs(t, err, sentinel)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestService_Delete_HappyPath(t *testing.T) {
	d := newAvailableDevice(t)
	var deleted uuid.UUID
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		DeleteFunc: func(_ context.Context, id uuid.UUID) error {
			deleted = id
			return nil
		},
	}
	svc := newService(repo)

	err := svc.Delete(context.Background(), d.ID())
	require.NoError(t, err)
	require.Equal(t, d.ID(), deleted)
}

func TestService_Delete_InUseBlocked(t *testing.T) {
	d := newInState(t, device.StateInUse)
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		DeleteFunc: func(context.Context, uuid.UUID) error {
			t.Fatalf("Repository.Delete must not be called for an in-use device")
			return nil
		},
	}
	svc := newService(repo)

	err := svc.Delete(context.Background(), d.ID())
	require.ErrorIs(t, err, device.ErrDeviceInUse)
}

func TestService_Delete_NotFound(t *testing.T) {
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) {
			return nil, device.ErrDeviceNotFound
		},
		DeleteFunc: func(context.Context, uuid.UUID) error {
			t.Fatalf("Repository.Delete must not be called when the device is not found")
			return nil
		},
	}
	svc := newService(repo)

	err := svc.Delete(context.Background(), uuid.New())
	require.ErrorIs(t, err, device.ErrDeviceNotFound)
}

func TestService_Delete_PropagatesRepoError(t *testing.T) {
	d := newAvailableDevice(t)
	sentinel := errors.New("boom")
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		DeleteFunc:  func(context.Context, uuid.UUID) error { return sentinel },
	}
	svc := newService(repo)

	err := svc.Delete(context.Background(), d.ID())
	require.ErrorIs(t, err, sentinel)
}
