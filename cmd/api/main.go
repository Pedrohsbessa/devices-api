// Binary devices-api serves the Devices REST API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Pedrohsbessa/devices-api/internal/device"
	"github.com/Pedrohsbessa/devices-api/internal/device/httpapi"
	"github.com/Pedrohsbessa/devices-api/internal/device/postgres"
	"github.com/Pedrohsbessa/devices-api/internal/platform/config"
	"github.com/Pedrohsbessa/devices-api/internal/platform/httpx"
	"github.com/Pedrohsbessa/devices-api/internal/platform/logger"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.Any("error", err))
		os.Exit(1)
	}
}

// run owns the full lifecycle of the service. Keeping the main function a
// thin wrapper around run that returns an error makes ordering and cleanup
// explicit — every resource acquired here is released via defer before
// returning, even on error paths.
func run() error {
	// 1. Configuration.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// 2. Logger — install as the default so any library that falls back
	// to slog.Default emits through our handler.
	log := logger.New(cfg.Log, os.Stdout)
	slog.SetDefault(log)
	log.Info("configuration loaded",
		slog.String("addr", cfg.HTTP.Addr),
		slog.String("log_level", cfg.Log.Level.String()),
		slog.String("log_format", cfg.Log.Format),
	)

	// 3. Signal-aware context: triggers the cancellation chain on
	// SIGINT (Ctrl+C) or SIGTERM (container orchestrator shutdown).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 4. Database pool. Deferred Close runs last, after the HTTP server
	// has drained in-flight requests.
	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("pgxpool: %w", err)
	}
	defer pool.Close()

	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := pool.Ping(pingCtx); err != nil {
		pingCancel()
		return fmt.Errorf("database ping: %w", err)
	}
	pingCancel()
	log.Info("database connection established")

	// 5. Wire dependencies top-down: repo → service → handler.
	repo := postgres.NewRepository(pool)
	svc := device.NewService(repo, time.Now)
	handler := httpapi.NewHandler(svc)

	// 6. Routes.
	mux := http.NewServeMux()
	handler.Routes(mux)
	mux.HandleFunc("GET /healthz", httpx.Healthz())
	mux.HandleFunc("GET /readyz", httpx.Readyz(pool))

	// 7. Middleware chain. Applied from inside out, so the outermost
	// here is RequestID — it runs first and gives every downstream
	// middleware a usable request id.
	var h http.Handler = mux
	h = httpx.Timeout(cfg.HTTP.RequestTimeout)(h)
	h = httpx.Recover(log)(h)
	h = httpx.Logger(log)(h)
	h = httpx.RequestID(h)

	// 8. HTTP server.
	srv := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      h,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	// 9. Serve until the context is cancelled or the server fails to
	// bind. The goroutine only reports non-shutdown errors.
	serveErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", slog.String("addr", cfg.HTTP.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
	}

	// 10. Graceful shutdown: drain in-flight requests first, then let
	// the deferred pool.Close run once the server is gone.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("http server shutdown error", slog.Any("error", err))
	}
	log.Info("http server stopped")
	return nil
}
