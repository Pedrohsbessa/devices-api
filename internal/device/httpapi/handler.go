package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"

	"github.com/Pedrohsbessa/devices-api/internal/device"
	"github.com/Pedrohsbessa/devices-api/internal/platform/httpx"
)

// maxBodyBytes caps the size of any request body accepted by the
// handlers. 64 KiB is several orders of magnitude beyond a well-formed
// device payload and provides a cheap defence against pathological
// clients.
const maxBodyBytes = 64 << 10

// Handler serves the device HTTP endpoints. Create one with NewHandler
// and mount its routes via Routes.
type Handler struct {
	svc *device.Service
}

// NewHandler wires an HTTP handler around a device Service.
func NewHandler(svc *device.Service) *Handler {
	return &Handler{svc: svc}
}

// Routes attaches every device endpoint to mux. Middlewares (request id,
// logging, recover, timeout) are the caller's responsibility: mount them
// outside, not here, so the same handler can be reused in tests with a
// bare mux.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /devices", h.create)
	mux.HandleFunc("GET /devices", h.list)
	mux.HandleFunc("GET /devices/{id}", h.get)
	mux.HandleFunc("PUT /devices/{id}", h.replace)
	mux.HandleFunc("PATCH /devices/{id}", h.patch)
	mux.HandleFunc("DELETE /devices/{id}", h.delete)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateDeviceRequest
	if !h.decodeBody(w, r, &req) {
		return
	}
	d, err := h.svc.Create(r.Context(), req.Name, req.Brand)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	w.Header().Set("Location", "/devices/"+d.ID().String())
	writeJSON(w, http.StatusCreated, deviceToResponse(d))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}
	d, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, deviceToResponse(d))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	filter, err := parseListFilter(r.URL.Query())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	devices, err := h.svc.List(r.Context(), filter)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	resp := ListDevicesResponse{
		Data:   make([]DeviceResponse, 0, len(devices)),
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}
	for _, d := range devices {
		resp.Data = append(resp.Data, deviceToResponse(d))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) replace(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}
	var req ReplaceDeviceRequest
	if !h.decodeBody(w, r, &req) {
		return
	}
	patch := device.UpdatePatch{
		Name:  &req.Name,
		Brand: &req.Brand,
		State: &req.State,
	}
	d, err := h.svc.Update(r.Context(), id, patch)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, deviceToResponse(d))
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}
	var req PatchDeviceRequest
	if !h.decodeBody(w, r, &req) {
		return
	}
	patch := device.UpdatePatch{
		Name:  req.Name,
		Brand: req.Brand,
		State: req.State,
	}
	d, err := h.svc.Update(r.Context(), id, patch)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, deviceToResponse(d))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (h *Handler) parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.writeProblem(w, r, httpx.Problem{
			Title:  "Bad Request",
			Status: http.StatusBadRequest,
			Detail: "id must be a valid UUID",
		})
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.writeProblem(w, r, httpx.Problem{
				Title:  "Payload Too Large",
				Status: http.StatusRequestEntityTooLarge,
				Detail: fmt.Sprintf("request body exceeds %d bytes", maxBodyBytes),
			})
			return false
		}
		h.writeProblem(w, r, httpx.Problem{
			Title:  "Bad Request",
			Status: http.StatusBadRequest,
			Detail: "invalid request body: " + err.Error(),
		})
		return false
	}
	if dec.More() {
		h.writeProblem(w, r, httpx.Problem{
			Title:  "Bad Request",
			Status: http.StatusBadRequest,
			Detail: "body must contain a single JSON object",
		})
		return false
	}
	return true
}

// parseListFilter translates query parameters into a device.ListFilter,
// returning wrapped ErrInvalidQuery (or ErrInvalidState) values so the
// error mapper produces 400 responses.
func parseListFilter(q url.Values) (device.ListFilter, error) {
	f := device.ListFilter{}
	if v := q.Get("brand"); v != "" {
		f.Brand = &v
	}
	if v := q.Get("state"); v != "" {
		s := device.State(v)
		if !s.Valid() {
			return f, fmt.Errorf("%w: %q", device.ErrInvalidState, v)
		}
		f.State = &s
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, fmt.Errorf("%w: limit must be a non-negative integer", ErrInvalidQuery)
		}
		f.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, fmt.Errorf("%w: offset must be a non-negative integer", ErrInvalidQuery)
		}
		f.Offset = n
	}
	return f, nil
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteProblem(w, ProblemFromError(err, r.URL.Path, httpx.RequestIDFrom(r.Context())))
}

func (h *Handler) writeProblem(w http.ResponseWriter, r *http.Request, p httpx.Problem) {
	p.Instance = r.URL.Path
	p.RequestID = httpx.RequestIDFrom(r.Context())
	httpx.WriteProblem(w, p)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
