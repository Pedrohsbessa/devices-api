// Package httpapi is the HTTP adapter for the device domain: handlers,
// DTOs and the domain-to-problem error mapping.
package httpapi

import (
	"errors"
	"net/http"

	"github.com/Pedrohsbessa/devices-api/internal/device"
	"github.com/Pedrohsbessa/devices-api/internal/platform/httpx"
)

// ProblemFromError translates a domain (or application) error into an
// RFC 7807 Problem. Known domain sentinels are mapped to specific
// status/title pairs with a client-friendly detail; anything else
// collapses to a 500 with a generic message so internal errors never
// leak implementation details to the caller.
func ProblemFromError(err error, instance, requestID string) httpx.Problem {
	p := httpx.Problem{
		Instance:  instance,
		RequestID: requestID,
	}
	switch {
	case errors.Is(err, device.ErrDeviceNotFound):
		p.Status = http.StatusNotFound
		p.Title = "Device Not Found"
		p.Detail = err.Error()
	case errors.Is(err, device.ErrDeviceInUse):
		p.Status = http.StatusConflict
		p.Title = "Device In Use"
		p.Detail = err.Error()
	case errors.Is(err, device.ErrInvalidState),
		errors.Is(err, device.ErrNameRequired),
		errors.Is(err, device.ErrBrandRequired):
		p.Status = http.StatusBadRequest
		p.Title = "Validation Error"
		p.Detail = err.Error()
	default:
		p.Status = http.StatusInternalServerError
		p.Title = "Internal Server Error"
		p.Detail = "unexpected server error"
	}
	return p
}
