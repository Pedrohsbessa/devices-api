package httpapi

import (
	"time"

	"github.com/Pedrohsbessa/devices-api/internal/device"
)

// CreateDeviceRequest is the body accepted by POST /devices. Both fields
// are semantically required; invalid values surface as domain validation
// errors returned by the service.
type CreateDeviceRequest struct {
	Name  string `json:"name"`
	Brand string `json:"brand"`
}

// ReplaceDeviceRequest is the body accepted by PUT /devices/{id}. Since
// PUT replaces the full representation, all three fields are required.
type ReplaceDeviceRequest struct {
	Name  string       `json:"name"`
	Brand string       `json:"brand"`
	State device.State `json:"state"`
}

// PatchDeviceRequest is the body accepted by PATCH /devices/{id}. Nil
// pointers mean "leave the field alone"; non-nil means "apply this value".
type PatchDeviceRequest struct {
	Name  *string       `json:"name,omitempty"`
	Brand *string       `json:"brand,omitempty"`
	State *device.State `json:"state,omitempty"`
}

// DeviceResponse is the representation of a Device sent to clients.
type DeviceResponse struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Brand     string       `json:"brand"`
	State     device.State `json:"state"`
	CreatedAt time.Time    `json:"created_at"`
}

// ListDevicesResponse wraps a page of DeviceResponse values and echoes
// the pagination inputs so clients can build next-page requests.
type ListDevicesResponse struct {
	Data   []DeviceResponse `json:"data"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

func deviceToResponse(d *device.Device) DeviceResponse {
	return DeviceResponse{
		ID:        d.ID().String(),
		Name:      d.Name(),
		Brand:     d.Brand(),
		State:     d.State(),
		CreatedAt: d.CreatedAt(),
	}
}
