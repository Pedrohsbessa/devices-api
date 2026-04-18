// Package logger builds a configured *slog.Logger from the project's
// LogConfig. It is a thin wrapper kept isolated so the config and main
// packages do not import slog directly.
package logger

import (
	"io"
	"log/slog"

	"github.com/Pedrohsbessa/devices-api/internal/platform/config"
)

// New returns a *slog.Logger that writes to w using the handler format
// and level described by cfg. Unknown formats fall back to JSON.
func New(cfg config.LogConfig, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.Level}
	var handler slog.Handler
	switch cfg.Format {
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default:
		handler = slog.NewJSONHandler(w, opts)
	}
	return slog.New(handler)
}
