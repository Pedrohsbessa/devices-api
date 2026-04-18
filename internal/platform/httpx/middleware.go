package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type ctxKey int

const requestIDKey ctxKey = iota

// HeaderRequestID is the canonical request-id header, read on the way in
// and written on the way out so upstream traces can correlate.
const HeaderRequestID = "X-Request-ID"

// RequestID reads X-Request-ID from the request or generates a new UUID
// when absent. The final value is placed on the response header and on
// the request context so downstream handlers and loggers can access it.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(HeaderRequestID, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFrom returns the request id stored in ctx, or "" if absent.
func RequestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// responseRecorder wraps http.ResponseWriter so the Logger middleware can
// observe status and byte count after the handler runs.
type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (rw *responseRecorder) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseRecorder) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

// Logger emits one structured line per request carrying method, path,
// status, bytes, duration and request id. It uses the provided slog
// logger so the host application controls format (JSON, text) and sink.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseRecorder{ResponseWriter: w}
			next.ServeHTTP(rw, r)

			logger.LogAttrs(r.Context(), slog.LevelInfo, "http_request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.status),
				slog.Int("bytes", rw.bytes),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", RequestIDFrom(r.Context())),
			)
		})
	}
}

// Recover catches panics raised by downstream handlers, logs them with
// the full value and returns a 500 problem+json. The stack is available
// via the slog handler if configured with AddSource.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.LogAttrs(r.Context(), slog.LevelError, "http_panic",
						slog.Any("panic", rec),
						slog.String("request_id", RequestIDFrom(r.Context())),
					)
					WriteProblem(w, Problem{
						Title:     "Internal Server Error",
						Status:    http.StatusInternalServerError,
						Detail:    "unexpected server error",
						Instance:  r.URL.Path,
						RequestID: RequestIDFrom(r.Context()),
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Timeout sets a deadline on the request context. Handlers that respect
// ctx.Done() — including the pgx pool used downstream — short-circuit
// when the deadline fires.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
