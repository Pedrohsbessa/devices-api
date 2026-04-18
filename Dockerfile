# syntax=docker/dockerfile:1.7

# -----------------------------------------------------------------------------
# Stage 1: build the static binary.
#
# The builder must match the Go toolchain our module requires (see the
# `go` directive in go.mod). Transitive dependencies in the testcontainers
# tree pin `go 1.25+`, so we build with 1.25.
# -----------------------------------------------------------------------------
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Cache the module download layer independently from the source so
# `go mod download` only reruns when go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source.
COPY . .

# CGO disabled + -trimpath + -ldflags "-s -w" yields a small, reproducible
# binary that runs on the empty distroless base.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN go build -trimpath -ldflags="-s -w" -o /out/devices-api ./cmd/api

# -----------------------------------------------------------------------------
# Stage 2: runtime.
#
# Distroless/static carries no shell, package manager or libc — just the
# binary and a non-root user. It is the right base for a pure-Go service
# because CGO is disabled, and it shrinks the attack surface to almost
# nothing (~2 MiB base).
# -----------------------------------------------------------------------------
FROM gcr.io/distroless/static:nonroot

LABEL org.opencontainers.image.source="https://github.com/Pedrohsbessa/devices-api"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.description="Devices REST API (Go 1.23 + PostgreSQL)"

COPY --from=builder /out/devices-api /usr/local/bin/devices-api

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/devices-api"]
