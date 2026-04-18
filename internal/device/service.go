package device

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Service orchestrates Repository access and the Device domain rules. It
// is the only layer that composes persistence with invariant checks — the
// HTTP layer above deals only with request/response shapes, and the
// Repository below is strictly an I/O adapter.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService wires a Service around a Repository and a time source. The
// clock is injected so tests can run with deterministic timestamps and
// the service does not import the wall clock directly.
func NewService(repo Repository, now func() time.Time) *Service {
	return &Service{repo: repo, now: now}
}

// UpdatePatch carries the fields a caller wants to change on a Device.
// Nil pointers mean "leave the field alone"; non-nil pointers mean "apply
// this value". The underlying value (including empty strings) is forwarded
// to the domain mutators, which are the ones that decide acceptability.
type UpdatePatch struct {
	Name  *string
	Brand *string
	State *State
}

// Create builds a new Device and persists it. Returns the persisted
// aggregate; validation errors from the domain surface unwrapped so the
// HTTP layer can map them with errors.Is.
func (s *Service) Create(ctx context.Context, name, brand string) (*Device, error) {
	d, err := NewDevice(name, brand, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// Get loads a Device by id, returning ErrDeviceNotFound when absent.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Device, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns devices matching the filter.
func (s *Service) List(ctx context.Context, f ListFilter) ([]*Device, error) {
	return s.repo.List(ctx, f)
}

// Update re-reads the aggregate, applies the patch on the in-memory
// instance and persists the result. Invariants are re-checked against the
// current persisted state, not against the caller's assumption — a client
// patching a device that was transitioned to in-use between its read and
// write receives ErrDeviceInUse, not a stale success.
//
// State transitions are applied first so a single PATCH can unlock the
// device and then rename it in the same request. The reverse order would
// still evaluate the rename against the (stale) in-use state.
func (s *Service) Update(ctx context.Context, id uuid.UUID, p UpdatePatch) (*Device, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.State != nil {
		if err := d.ChangeState(*p.State); err != nil {
			return nil, err
		}
	}
	if p.Name != nil {
		if err := d.Rename(*p.Name); err != nil {
			return nil, err
		}
	}
	if p.Brand != nil {
		if err := d.ChangeBrand(*p.Brand); err != nil {
			return nil, err
		}
	}
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// Delete removes a device after confirming the in-use rule. A device in
// the StateInUse transition is rejected with ErrDeviceInUse; a missing
// device is rejected with ErrDeviceNotFound.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := d.CanDelete(); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}
