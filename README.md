# Devices API

[![CI](https://github.com/Pedrohsbessa/devices-api/actions/workflows/ci.yml/badge.svg)](https://github.com/Pedrohsbessa/devices-api/actions/workflows/ci.yml)
[![Go Report](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)

A REST API for managing device resources, written in Go with PostgreSQL.

---

## Table of contents

- [Highlights](#highlights)
- [Quickstart (Docker Compose)](#quickstart-docker-compose)
- [Development without Docker](#development-without-docker)
- [API reference](#api-reference)
- [Architecture](#architecture)
- [Testing](#testing)
- [IDE setup](#ide-setup)
- [Makefile](#makefile)
- [Design decisions](#design-decisions)
- [Future improvements](#future-improvements)
- [License](#license)

---

## Highlights

- Go 1.25 — stdlib `net/http` with method+path routing, no framework.
- PostgreSQL 16 via `jackc/pgx/v5` — no ORM, prepared statements, pooled connections.
- Rich domain model: unexported fields, invariants enforced by methods at compile time.
- RFC 7807 `application/problem+json` error responses with `request_id` extension.
- OpenAPI 3.1 contract and Redoc UI served by the binary (`go:embed`, zero runtime file access).
- Structured `slog` JSON logging, request id propagation, graceful shutdown via `signal.NotifyContext`.
- Four-tier test suite: domain unit, service unit with hand-written mock, HTTP black-box, repository integration against a real Postgres via `testcontainers-go`.
- Multi-stage Docker build on `distroless/static:nonroot` — **~20 MiB final image**, non-root user, `CGO_ENABLED=0`.
- GitHub Actions CI: lint, unit, integration, binary build, Docker build — parallel, ~50 s total.

---

## Quickstart (Docker Compose)

```bash
cp .env.example .env
docker compose up --build
```

The stack lifts in this order: `postgres` becomes healthy → `migrate` applies every migration once and exits → `api` starts only after the schema is ready.

Open the API once it is up:

| URL                                      | What it is                           |
|------------------------------------------|--------------------------------------|
| http://localhost:8080/docs               | Interactive reference (Redoc)        |
| http://localhost:8080/openapi.yaml       | OpenAPI 3.1 contract (YAML)          |
| http://localhost:8080/healthz            | Liveness probe                       |
| http://localhost:8080/readyz             | Readiness probe (pings Postgres)     |
| http://localhost:8080/devices            | Device resource                      |

Tear everything down:

```bash
docker compose down      # keeps the postgres volume
docker compose down -v   # drops the volume too
```

---

## Development without Docker

**Requirements**

- Go 1.25+
- Docker (for Postgres and integration tests)
- `make`

**Start Postgres in a container**

```bash
docker run -d --rm --name devices-pg \
  -e POSTGRES_USER=devices \
  -e POSTGRES_PASSWORD=devices \
  -e POSTGRES_DB=devices \
  -p 5432:5432 \
  postgres:16-alpine
```

**Apply migrations and run the API**

```bash
export DATABASE_URL="postgres://devices:devices@localhost:5432/devices?sslmode=disable"
make migrate-up
make run
```

The API serves on `http://localhost:8080`. Press `Ctrl+C` for a graceful shutdown (drains in-flight requests, closes the pool).

---

## API reference

| Method | Path                | Success        | Notes                                    |
|--------|---------------------|----------------|------------------------------------------|
| POST   | `/devices`          | 201 Created    | `Location: /devices/{id}` header         |
| GET    | `/devices`          | 200 OK         | Query: `brand`, `state`, `limit`, `offset` |
| GET    | `/devices/{id}`     | 200 OK         |                                          |
| PUT    | `/devices/{id}`     | 200 OK         | Replaces the full representation         |
| PATCH  | `/devices/{id}`     | 200 OK         | Partial update; state applied first      |
| DELETE | `/devices/{id}`     | 204 No Content |                                          |
| GET    | `/healthz`          | 200 OK         | Liveness                                 |
| GET    | `/readyz`           | 200 / 503      | Readiness; 503 if DB unreachable         |
| GET    | `/openapi.yaml`     | 200 OK         | `application/yaml`                       |
| GET    | `/docs`             | 200 OK         | Redoc UI                                 |

### Error responses

Every 4xx/5xx uses `application/problem+json` (RFC 7807):

```json
{
  "type": "about:blank",
  "title": "Device In Use",
  "status": 409,
  "detail": "device is in use",
  "instance": "/devices/019da2aa-0c40-781e-a049-2e0a968a10a6",
  "request_id": "3a3434ab-8475-42ba-8ea2-1ab55b5e86be"
}
```

`request_id` mirrors the `X-Request-ID` response header — the same id appears in the server log for correlation.

### curl examples

Create, capture id, then drive a few scenarios:

```bash
# Create
ID=$(curl -sX POST http://localhost:8080/devices \
  -H "Content-Type: application/json" \
  -d '{"name":"ThinkPad X1","brand":"Lenovo"}' | jq -r .id)

# Fetch
curl -s http://localhost:8080/devices/$ID | jq

# List with filters
curl -s "http://localhost:8080/devices?brand=Lenovo&state=available&limit=10" | jq

# Put a device in use
curl -sX PATCH http://localhost:8080/devices/$ID \
  -H "Content-Type: application/json" \
  -d '{"state":"in-use"}' | jq

# Try to delete while in-use -> 409 Conflict (problem+json)
curl -sX DELETE http://localhost:8080/devices/$ID | jq

# Unlock and rename in a single PATCH (state change is applied before rename)
curl -sX PATCH http://localhost:8080/devices/$ID \
  -H "Content-Type: application/json" \
  -d '{"state":"available","name":"Renamed"}' | jq

# Delete
curl -sX DELETE -w "%{http_code}\n" http://localhost:8080/devices/$ID
```

---

## Architecture

```
cmd/api/                    # main and run(): config, pool, mux, server, shutdown
internal/device/            # core domain
  device.go                 #   aggregate with unexported fields + mutators
  state.go                  #   State value object + (Un)MarshalJSON
  errors.go                 #   typed domain sentinels
  service.go                #   orchestration: read-before-mutate
  repository.go             #   Repository interface, ListFilter
internal/device/postgres/   # adapter: pgx pool implementation of Repository
internal/device/httpapi/    # adapter: handlers, DTOs, problem+json mapping
internal/platform/          # cross-cutting, domain-agnostic
  config/                   #   env-driven configuration with errors.Join
  logger/                   #   slog JSON/text handler
  httpx/                    #   RFC 7807 helpers, middlewares, healthz/readyz, docs handlers
api/                        # OpenAPI contract + Redoc HTML (embedded via go:embed)
migrations/                 # golang-migrate SQL files
```

**Dependencies point from specific to generic.** `httpapi` imports `device` and `httpx`; `postgres` imports `device`; `httpx` imports nothing of the domain. `device` has no adapters.

**The service mediates between HTTP and persistence.** Handlers are a thin shell that parses requests, invokes a single service method, and translates errors via `ProblemFromError`. The service re-reads the aggregate before mutations so invariants are checked against the persisted state, not the client's snapshot.

---

## Testing

Four layers, all exercised in CI:

| Layer                  | Command                      | Scope                                                     | Typical time |
|------------------------|------------------------------|-----------------------------------------------------------|--------------|
| Domain unit            | `make test`                  | entity, state, invariants, mutators                       | < 2 s        |
| Service unit           | `make test`                  | service with a hand-written repository stub               | < 2 s        |
| HTTP black-box         | `make test`                  | real service + mux, `httptest` request/response           | < 2 s        |
| Repository integration | `make test-integration`      | pgx adapter against a real Postgres (testcontainers)     | ~8 s         |

Coverage snapshot on the main branch:

- `internal/device` — 97,6 % of statements
- `internal/device/httpapi` — 92,4 % of statements
- `internal/device/postgres` — 100 % of functional paths (integration)

Run everything locally:

```bash
make test
make test-integration
```

Integration tests are behind the `integration` build tag (`//go:build integration`) so the unit suite stays Docker-free.

---

## IDE setup

Integration tests live behind the `integration` build tag. Default `go test` and IDE test runners don't see them until the tag is enabled.

### VS Code

Create `.vscode/settings.json`:

```json
{
  "go.buildTags": "integration",
  "go.testTags": "integration",
  "go.testTimeout": "120s"
}
```

`.vscode/` is already in `.gitignore`, so this file stays local to the developer.

### GoLand / IntelliJ

Preferences → Go → **Build Tags & Vendoring** → Custom tags: `integration`.

### Zed

Add to `.zed/settings.json`:

```json
{
  "lsp": {
    "gopls": {
      "initialization_options": {
        "buildFlags": ["-tags=integration"]
      }
    }
  }
}
```

### Terminal only

```bash
make test-integration
# or
go test -tags=integration -race ./internal/device/postgres/...
```

---

## Makefile

`make help` prints every target. The most used:

| Target                | What it does                                                      |
|-----------------------|-------------------------------------------------------------------|
| `make run`            | Run the API locally (reads `DATABASE_URL`)                        |
| `make build`          | Compile a trimmed, stripped, static binary into `./bin`           |
| `make test`           | Unit tests with race detector, `-short`                           |
| `make test-integration` | Integration tests with the `integration` build tag              |
| `make test-cover`     | Coverage profile and summary                                      |
| `make lint`           | golangci-lint v2                                                  |
| `make migrate-up`     | Apply all pending migrations                                      |
| `make migrate-down`   | Revert the last applied migration                                 |
| `make migrate-create name=<name>` | Create a new migration pair                           |
| `make docker-up`      | Start the compose stack in the background                         |
| `make docker-down`    | Stop the compose stack (keeps volumes)                            |
| `make docker-logs`    | Tail logs from every service                                      |

---

## Design decisions

Short rationale for the non-obvious choices. Per-commit context is in the git log.

- **UUIDv7 ids.** Timestamp-ordered, so `ORDER BY id DESC` replaces an index on `created_at`. Inserts hit the B-tree sequentially, reducing fragmentation.
- **Unexported struct fields.** Invariants like "creation time is immutable" and "name/brand locked while in-use" cannot be bypassed from outside the package — the compiler refuses the assignment.
- **`Reconstruct` constructor for the repository.** Separates validated entry (`NewDevice`) from trusted re-hydration (`Reconstruct` with DB rows).
- **Timestamp truncated to microseconds in `NewDevice`.** PostgreSQL `TIMESTAMPTZ` stores microsecond precision; Go's `time.Now()` exposes nanoseconds. Aligning in the domain makes round-trips exact on every platform.
- **Re-read before mutation in the service.** Invariants are checked against the current persisted state, not against whatever the client thought was true at read time.
- **State-first ordering in PATCH.** `ChangeState` runs before `Rename` / `ChangeBrand`, so a single `PATCH` can unlock an `in-use` device and rewrite its metadata — the common operational flow.
- **One endpoint for listing with filters.** `GET /devices?brand=...&state=...` covers "fetch all", "by brand" and "by state" with one route. Bitmap-AND in the planner handles combined filters; a composite index is a later optimisation.
- **Typed domain errors + RFC 7807 mapper on the edge.** `errors.Is` works across wraps. The HTTP layer is the only place that knows HTTP status codes.
- **Re-usable `httpx` platform package.** `Problem`, middlewares, health probes and docs handlers know nothing about devices. A second resource reuses them without refactor.
- **`DisallowUnknownFields` + `MaxBytesReader(64 KiB)` on every body.** Typos become 400 instead of silent partial updates. Oversized bodies cut off at TCP level via `MaxBytesReader(w, ...)`.
- **`application/problem+json` with `request_id` extension.** Clients forward the id in support tickets; operators grep logs directly.
- **`net/http` stdlib + ServeMux.** Go 1.22+ routes by method and path, yields 405 automatically, and exposes path variables via `r.PathValue`. No router dependency.
- **TEXT + CHECK for enum columns, not native ENUM.** CHECK constraints evolve with a simple migration; ENUMs need `ALTER TYPE` and cannot drop in-use values.
- **CGO_ENABLED=0 on distroless/static:nonroot.** ~20 MiB image, no shell, non-root by default. Smallest operational surface for a Go binary.
- **Migrations as a one-shot compose service.** Idempotent and safe under horizontal scaling — the API never races itself to migrate.

---

## Future improvements

Deferred intentionally to keep scope focused. Each is a clean, additive change:

- `updated_at` column plus optimistic concurrency (`ETag` + `If-Match` or a version column).
- Composite index `(brand, state)` when profiling identifies the combined filter as a hot path.
- Cursor-based pagination once offset costs begin to matter on large tables.
- Authentication and authorization (JWT or API key) plus rate limiting.
- Prometheus metrics at `/metrics` and OpenTelemetry tracing.
- Audit log of state transitions.
- Domain events (outbox pattern) for integration with downstream systems.
- Read cache (Redis) for `GET by id`.
- Contract tests (Schemathesis or Pact) against the OpenAPI document.
- Multi-arch Docker images (linux/amd64 + linux/arm64) via `buildx`.

---

## License

[MIT](./LICENSE)
