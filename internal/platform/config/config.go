// Package config loads and validates application configuration from
// environment variables. All values have sensible defaults except the
// database URL, which is the one required input.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Config is the root configuration for the service.
type Config struct {
	HTTP     HTTPConfig
	Database DatabaseConfig
	Log      LogConfig
}

// HTTPConfig carries server timeouts and the listen address.
type HTTPConfig struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

// DatabaseConfig carries the connection URL for PostgreSQL.
type DatabaseConfig struct {
	URL string
}

// LogConfig carries log level and output format.
type LogConfig struct {
	Level  slog.Level
	Format string // "json" or "text"
}

// Load reads configuration from environment variables, returning a
// validated Config. Missing or invalid values are collected and returned
// together via errors.Join so callers can see every problem at once.
func Load() (*Config, error) {
	var errs []error

	cfg := &Config{
		HTTP: HTTPConfig{
			Addr:            getEnv("HTTP_ADDR", ":8080"),
			ReadTimeout:     getDuration("HTTP_READ_TIMEOUT", 10*time.Second, &errs),
			WriteTimeout:    getDuration("HTTP_WRITE_TIMEOUT", 10*time.Second, &errs),
			IdleTimeout:     getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second, &errs),
			RequestTimeout:  getDuration("HTTP_REQUEST_TIMEOUT", 15*time.Second, &errs),
			ShutdownTimeout: getDuration("SHUTDOWN_TIMEOUT", 30*time.Second, &errs),
		},
		Database: DatabaseConfig{
			URL: os.Getenv("DATABASE_URL"),
		},
		Log: LogConfig{
			Level:  parseLogLevel(getEnv("LOG_LEVEL", "info"), &errs),
			Format: getEnv("LOG_FORMAT", "json"),
		},
	}

	if cfg.Database.URL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if cfg.Log.Format != "json" && cfg.Log.Format != "text" {
		errs = append(errs, fmt.Errorf("LOG_FORMAT must be json or text, got %q", cfg.Log.Format))
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getDuration(key string, def time.Duration, errs *[]error) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: invalid duration %q: %w", key, v, err))
		return def
	}
	return d
}

func parseLogLevel(s string, errs *[]error) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		*errs = append(*errs, fmt.Errorf("LOG_LEVEL must be debug/info/warn/error, got %q", s))
		return slog.LevelInfo
	}
}
