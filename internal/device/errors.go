package device

import "errors"

// Sentinel errors for domain rule violations. Callers match specific failure
// modes with errors.Is; the HTTP adapter maps each one to the appropriate
// status code at the service boundary.
var (
	// ErrNameRequired is returned when a device is created or updated with
	// an empty or whitespace-only name.
	ErrNameRequired = errors.New("device name is required")

	// ErrBrandRequired is returned when a device is created or updated with
	// an empty or whitespace-only brand.
	ErrBrandRequired = errors.New("device brand is required")

	// ErrInvalidState is returned when a value is not one of the three known
	// device states (available, in-use, inactive).
	ErrInvalidState = errors.New("invalid device state")

	// ErrDeviceNotFound is returned when no device exists for a given id.
	// The HTTP adapter maps it to 404 Not Found.
	ErrDeviceNotFound = errors.New("device not found")

	// ErrDeviceInUse is returned when an operation is blocked because the
	// device is in use: name and brand cannot be updated, and the device
	// cannot be deleted. The HTTP adapter maps it to 409 Conflict.
	ErrDeviceInUse = errors.New("device is in use")
)
