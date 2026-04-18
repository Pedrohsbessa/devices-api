package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Pedrohsbessa/devices-api/internal/device"
	"github.com/Pedrohsbessa/devices-api/internal/device/httpapi"
	"github.com/Pedrohsbessa/devices-api/internal/platform/httpx"
)

// ---------------------------------------------------------------------------
// Fixtures and stubs
// ---------------------------------------------------------------------------

var fixedNow = time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)

// repoStub is a tiny implementation of device.Repository. Tests set only
// the hooks they expect to use; unset hooks panic with a clear stack
// trace when invoked unexpectedly.
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

var _ device.Repository = (*repoStub)(nil)

func newServer(t *testing.T, repo *repoStub) http.Handler {
	t.Helper()
	svc := device.NewService(repo, func() time.Time { return fixedNow })
	h := httpapi.NewHandler(svc)
	mux := http.NewServeMux()
	h.Routes(mux)
	return mux
}

func mustNewDevice(t *testing.T, name, brand string) *device.Device {
	t.Helper()
	d, err := device.NewDevice(name, brand, fixedNow)
	require.NoError(t, err)
	return d
}

func mustInUseDevice(t *testing.T) *device.Device {
	t.Helper()
	d := mustNewDevice(t, "laptop", "lenovo")
	require.NoError(t, d.ChangeState(device.StateInUse))
	return d
}

// do performs a request against handler and returns the recorded response.
func do(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) httpx.Problem {
	t.Helper()
	require.Equal(t, httpx.ProblemContentType, rec.Header().Get("Content-Type"))
	var p httpx.Problem
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&p))
	require.Equal(t, rec.Code, p.Status, "Problem.status must mirror the response code")
	return p
}

func decodeDevice(t *testing.T, rec *httptest.ResponseRecorder) httpapi.DeviceResponse {
	t.Helper()
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	var d httpapi.DeviceResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&d))
	return d
}

// ---------------------------------------------------------------------------
// POST /devices
// ---------------------------------------------------------------------------

func TestHandler_Create_Success(t *testing.T) {
	var captured *device.Device
	repo := &repoStub{
		CreateFunc: func(_ context.Context, d *device.Device) error {
			captured = d
			return nil
		},
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodPost, "/devices", `{"name":"laptop","brand":"lenovo"}`)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.NotNil(t, captured)
	require.Equal(t, "/devices/"+captured.ID().String(), rec.Header().Get("Location"))

	resp := decodeDevice(t, rec)
	require.Equal(t, captured.ID().String(), resp.ID)
	require.Equal(t, "laptop", resp.Name)
	require.Equal(t, "lenovo", resp.Brand)
	require.Equal(t, device.StateAvailable, resp.State)
	require.True(t, resp.CreatedAt.Equal(fixedNow))
}

func TestHandler_Create_ValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantTitle  string
		wantDetail string
	}{
		{"empty name", `{"name":"","brand":"lenovo"}`, "Validation Error", "name"},
		{"empty brand", `{"name":"laptop","brand":""}`, "Validation Error", "brand"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &repoStub{
				CreateFunc: func(context.Context, *device.Device) error {
					t.Fatalf("Repository.Create must not be called when domain validation fails")
					return nil
				},
			}
			handler := newServer(t, repo)

			rec := do(t, handler, http.MethodPost, "/devices", tt.body)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			p := decodeProblem(t, rec)
			require.Equal(t, tt.wantTitle, p.Title)
			require.Contains(t, p.Detail, tt.wantDetail)
		})
	}
}

func TestHandler_Create_MalformedBody(t *testing.T) {
	handler := newServer(t, &repoStub{})

	rec := do(t, handler, http.MethodPost, "/devices", `{invalid json`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	p := decodeProblem(t, rec)
	require.Equal(t, "Bad Request", p.Title)
}

func TestHandler_Create_UnknownFieldRejected(t *testing.T) {
	handler := newServer(t, &repoStub{})

	rec := do(t, handler, http.MethodPost, "/devices",
		`{"name":"x","brand":"y","typo":"bar"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	p := decodeProblem(t, rec)
	require.Contains(t, p.Detail, "unknown field")
}

func TestHandler_Create_PayloadTooLarge(t *testing.T) {
	handler := newServer(t, &repoStub{})

	big := fmt.Sprintf(`{"name":%q,"brand":"lenovo"}`, strings.Repeat("x", 70*1024))
	rec := do(t, handler, http.MethodPost, "/devices", big)

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	p := decodeProblem(t, rec)
	require.Equal(t, "Payload Too Large", p.Title)
}

func TestHandler_Create_RepoError(t *testing.T) {
	sentinel := errors.New("db down")
	repo := &repoStub{
		CreateFunc: func(context.Context, *device.Device) error { return sentinel },
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodPost, "/devices", `{"name":"x","brand":"y"}`)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	p := decodeProblem(t, rec)
	require.Equal(t, "Internal Server Error", p.Title)
	require.NotContains(t, p.Detail, "db down", "internal error detail must not leak")
}

// ---------------------------------------------------------------------------
// GET /devices/{id}
// ---------------------------------------------------------------------------

func TestHandler_Get_Success(t *testing.T) {
	d := mustNewDevice(t, "laptop", "lenovo")
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodGet, "/devices/"+d.ID().String(), "")

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeDevice(t, rec)
	require.Equal(t, d.ID().String(), resp.ID)
}

func TestHandler_Get_NotFound(t *testing.T) {
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) {
			return nil, device.ErrDeviceNotFound
		},
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodGet, "/devices/"+uuid.NewString(), "")

	require.Equal(t, http.StatusNotFound, rec.Code)
	p := decodeProblem(t, rec)
	require.Equal(t, "Device Not Found", p.Title)
}

func TestHandler_Get_InvalidUUID(t *testing.T) {
	handler := newServer(t, &repoStub{})

	rec := do(t, handler, http.MethodGet, "/devices/not-a-uuid", "")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	p := decodeProblem(t, rec)
	require.Contains(t, p.Detail, "UUID")
}

// ---------------------------------------------------------------------------
// GET /devices
// ---------------------------------------------------------------------------

func TestHandler_List_Empty(t *testing.T) {
	repo := &repoStub{
		ListFunc: func(context.Context, device.ListFilter) ([]*device.Device, error) {
			return nil, nil
		},
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodGet, "/devices", "")

	require.Equal(t, http.StatusOK, rec.Code)
	var resp httpapi.ListDevicesResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Empty(t, resp.Data)
}

func TestHandler_List_ForwardsFilter(t *testing.T) {
	var captured device.ListFilter
	repo := &repoStub{
		ListFunc: func(_ context.Context, f device.ListFilter) ([]*device.Device, error) {
			captured = f
			return nil, nil
		},
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodGet,
		"/devices?brand=lenovo&state=in-use&limit=10&offset=5", "")

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, captured.Brand)
	require.Equal(t, "lenovo", *captured.Brand)
	require.NotNil(t, captured.State)
	require.Equal(t, device.StateInUse, *captured.State)
	require.Equal(t, 10, captured.Limit)
	require.Equal(t, 5, captured.Offset)
}

func TestHandler_List_InvalidStateQuery(t *testing.T) {
	repo := &repoStub{
		ListFunc: func(context.Context, device.ListFilter) ([]*device.Device, error) {
			t.Fatalf("List must not be called when query validation fails")
			return nil, nil
		},
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodGet, "/devices?state=broken", "")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	p := decodeProblem(t, rec)
	require.Contains(t, p.Detail, "broken")
}

func TestHandler_List_InvalidLimitQuery(t *testing.T) {
	handler := newServer(t, &repoStub{})

	rec := do(t, handler, http.MethodGet, "/devices?limit=abc", "")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	p := decodeProblem(t, rec)
	require.Contains(t, p.Detail, "limit")
}

// ---------------------------------------------------------------------------
// PUT /devices/{id}
// ---------------------------------------------------------------------------

func TestHandler_Put_Success(t *testing.T) {
	d := mustNewDevice(t, "laptop", "lenovo")
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		UpdateFunc:  func(context.Context, *device.Device) error { return nil },
	}
	handler := newServer(t, repo)

	body := `{"name":"pro","brand":"dell","state":"inactive"}`
	rec := do(t, handler, http.MethodPut, "/devices/"+d.ID().String(), body)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeDevice(t, rec)
	require.Equal(t, "pro", resp.Name)
	require.Equal(t, "dell", resp.Brand)
	require.Equal(t, device.StateInactive, resp.State)
}

func TestHandler_Put_InUseConflict(t *testing.T) {
	d := mustInUseDevice(t)
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		UpdateFunc: func(context.Context, *device.Device) error {
			t.Fatalf("Repository.Update must not be called when in-use blocks the change")
			return nil
		},
	}
	handler := newServer(t, repo)

	// PUT carries every field; state stays in-use so Rename must be blocked.
	body := `{"name":"pro","brand":"dell","state":"in-use"}`
	rec := do(t, handler, http.MethodPut, "/devices/"+d.ID().String(), body)

	require.Equal(t, http.StatusConflict, rec.Code)
	p := decodeProblem(t, rec)
	require.Equal(t, "Device In Use", p.Title)
}

func TestHandler_Put_NotFound(t *testing.T) {
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) {
			return nil, device.ErrDeviceNotFound
		},
	}
	handler := newServer(t, repo)

	body := `{"name":"pro","brand":"dell","state":"available"}`
	rec := do(t, handler, http.MethodPut, "/devices/"+uuid.NewString(), body)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ---------------------------------------------------------------------------
// PATCH /devices/{id}
// ---------------------------------------------------------------------------

func TestHandler_Patch_SingleField(t *testing.T) {
	d := mustNewDevice(t, "laptop", "lenovo")
	var persisted *device.Device
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		UpdateFunc:  func(_ context.Context, d *device.Device) error { persisted = d; return nil },
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodPatch, "/devices/"+d.ID().String(), `{"name":"pro"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "pro", persisted.Name())
	require.Equal(t, "lenovo", persisted.Brand(), "other fields must not change")
}

// TestHandler_Patch_UnlockAndRename verifies the full HTTP -> service
// ordering contract: PATCHing an in-use device with {state, name}
// succeeds because the service applies ChangeState before Rename.
func TestHandler_Patch_UnlockAndRename(t *testing.T) {
	d := mustInUseDevice(t)
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		UpdateFunc:  func(context.Context, *device.Device) error { return nil },
	}
	handler := newServer(t, repo)

	body := `{"state":"available","name":"unlocked"}`
	rec := do(t, handler, http.MethodPatch, "/devices/"+d.ID().String(), body)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeDevice(t, rec)
	require.Equal(t, device.StateAvailable, resp.State)
	require.Equal(t, "unlocked", resp.Name)
}

func TestHandler_Patch_InUseConflict(t *testing.T) {
	d := mustInUseDevice(t)
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		UpdateFunc: func(context.Context, *device.Device) error {
			t.Fatalf("Repository.Update must not be called for rejected patches")
			return nil
		},
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodPatch, "/devices/"+d.ID().String(), `{"name":"x"}`)

	require.Equal(t, http.StatusConflict, rec.Code)
	p := decodeProblem(t, rec)
	require.Equal(t, "Device In Use", p.Title)
}

func TestHandler_Patch_NotFound(t *testing.T) {
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) {
			return nil, device.ErrDeviceNotFound
		},
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodPatch, "/devices/"+uuid.NewString(), `{"name":"x"}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ---------------------------------------------------------------------------
// DELETE /devices/{id}
// ---------------------------------------------------------------------------

func TestHandler_Delete_Success(t *testing.T) {
	d := mustNewDevice(t, "laptop", "lenovo")
	var deletedID uuid.UUID
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		DeleteFunc: func(_ context.Context, id uuid.UUID) error {
			deletedID = id
			return nil
		},
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodDelete, "/devices/"+d.ID().String(), "")

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String(), "204 responses must have no body")
	require.Equal(t, d.ID(), deletedID)
}

func TestHandler_Delete_InUseConflict(t *testing.T) {
	d := mustInUseDevice(t)
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) { return d, nil },
		DeleteFunc: func(context.Context, uuid.UUID) error {
			t.Fatalf("Repository.Delete must not be called for in-use devices")
			return nil
		},
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodDelete, "/devices/"+d.ID().String(), "")

	require.Equal(t, http.StatusConflict, rec.Code)
	p := decodeProblem(t, rec)
	require.Equal(t, "Device In Use", p.Title)
}

func TestHandler_Delete_NotFound(t *testing.T) {
	repo := &repoStub{
		GetByIDFunc: func(context.Context, uuid.UUID) (*device.Device, error) {
			return nil, device.ErrDeviceNotFound
		},
	}
	handler := newServer(t, repo)

	rec := do(t, handler, http.MethodDelete, "/devices/"+uuid.NewString(), "")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_Delete_InvalidUUID(t *testing.T) {
	handler := newServer(t, &repoStub{})

	rec := do(t, handler, http.MethodDelete, "/devices/not-a-uuid", "")
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------------------------------------------------------------------------
// Cross-cutting
// ---------------------------------------------------------------------------

// TestHandler_MethodNotAllowed exercises the ServeMux built-in: hitting
// a path with a method not registered should yield 405, not 404.
func TestHandler_MethodNotAllowed(t *testing.T) {
	handler := newServer(t, &repoStub{})

	rec := do(t, handler, http.MethodPost, "/devices/"+uuid.NewString(), `{"name":"x"}`)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
