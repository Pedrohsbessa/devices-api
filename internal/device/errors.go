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
)
