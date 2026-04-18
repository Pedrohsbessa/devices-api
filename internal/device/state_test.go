package device_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Pedrohsbessa/devices-api/internal/device"
)

func TestState_Valid(t *testing.T) {
	tests := []struct {
		name  string
		state device.State
		want  bool
	}{
		{"available", device.StateAvailable, true},
		{"in-use", device.StateInUse, true},
		{"inactive", device.StateInactive, true},
		{"empty", device.State(""), false},
		{"uppercase is not canonical", device.State("AVAILABLE"), false},
		{"unknown value", device.State("broken"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.state.Valid())
		})
	}
}

func TestState_String(t *testing.T) {
	require.Equal(t, "available", device.StateAvailable.String())
	require.Equal(t, "in-use", device.StateInUse.String())
	require.Equal(t, "inactive", device.StateInactive.String())
}

func TestState_MarshalJSON(t *testing.T) {
	t.Run("valid state emits canonical string", func(t *testing.T) {
		b, err := json.Marshal(device.StateAvailable)
		require.NoError(t, err)
		require.Equal(t, `"available"`, string(b))
	})

	t.Run("invalid state wraps ErrInvalidState", func(t *testing.T) {
		_, err := json.Marshal(device.State("broken"))
		require.Error(t, err)
		require.ErrorIs(t, err, device.ErrInvalidState)
	})
}

func TestState_UnmarshalJSON(t *testing.T) {
	validCases := []struct {
		raw  string
		want device.State
	}{
		{`"available"`, device.StateAvailable},
		{`"in-use"`, device.StateInUse},
		{`"inactive"`, device.StateInactive},
	}
	for _, tt := range validCases {
		t.Run("accepts "+tt.raw, func(t *testing.T) {
			var got device.State
			require.NoError(t, json.Unmarshal([]byte(tt.raw), &got))
			require.Equal(t, tt.want, got)
		})
	}

	invalidStrings := []string{`"broken"`, `""`, `"Available"`, `"AVAILABLE"`}
	for _, raw := range invalidStrings {
		t.Run("rejects "+raw, func(t *testing.T) {
			var got device.State
			err := json.Unmarshal([]byte(raw), &got)
			require.ErrorIs(t, err, device.ErrInvalidState)
		})
	}

	t.Run("non-string payload does not surface ErrInvalidState", func(t *testing.T) {
		var got device.State
		err := json.Unmarshal([]byte(`123`), &got)
		require.Error(t, err)
		require.False(t, errors.Is(err, device.ErrInvalidState),
			"type errors should remain type errors, not be classified as invalid state")
	})
}
