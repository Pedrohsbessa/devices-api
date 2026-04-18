package httpx

import (
	"context"
	"net/http"
	"time"
)

// Pinger is the minimal interface the readiness probe expects from a
// dependency. *pgxpool.Pool satisfies it via its Ping method.
type Pinger interface {
	Ping(ctx context.Context) error
}

const readyProbeTimeout = 2 * time.Second

// Healthz is a liveness probe: it returns 200 as long as the process is
// running and able to serve HTTP. No downstream dependencies are checked.
func Healthz() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}

// Readyz is a readiness probe: it returns 200 only when every provided
// dependency pings successfully within a short timeout. A failure
// produces a 503 problem+json so orchestrators (e.g. Kubernetes) drain
// traffic until the dependency comes back.
func Readyz(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyProbeTimeout)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			WriteProblem(w, Problem{
				Title:  "Service Unavailable",
				Status: http.StatusServiceUnavailable,
				Detail: "database unreachable",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}
}
