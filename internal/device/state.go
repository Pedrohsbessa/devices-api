// Package device is the core domain of the service. It holds the Device
// aggregate, its value objects and the rules that govern them.
//
// Entity fields are unexported on purpose: mutation can only happen through
// methods defined in this package, which guarantees that domain invariants
// (creation-time immutability, in-use restrictions) cannot be violated by
// external code.
package device

import (
	"encoding/json"
	"fmt"
)

// State is the lifecycle state of a Device. Only the three constants below
// are valid values; any other input is rejected at parse time.
type State string

// Canonical Device states. Any other value is rejected at parse time.
const (
	StateAvailable State = "available"
	StateInUse     State = "in-use"
	StateInactive  State = "inactive"
)

// Valid reports whether s is one of the known states.
func (s State) Valid() bool {
	switch s {
	case StateAvailable, StateInUse, StateInactive:
		return true
	default:
		return false
	}
}

// String implements fmt.Stringer.
func (s State) String() string { return string(s) }

// MarshalJSON emits the canonical string form. An attempt to marshal an
// unknown state is treated as a programming error — returning an error here
// prevents corrupted values from silently reaching clients.
func (s State) MarshalJSON() ([]byte, error) {
	if !s.Valid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidState, string(s))
	}
	return json.Marshal(string(s))
}

// UnmarshalJSON accepts only the three canonical strings (available, in-use,
// inactive) and rejects anything else by wrapping ErrInvalidState.
func (s *State) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("state must be a JSON string: %w", err)
	}
	candidate := State(raw)
	if !candidate.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidState, raw)
	}
	*s = candidate
	return nil
}
