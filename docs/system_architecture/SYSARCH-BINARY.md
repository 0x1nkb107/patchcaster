---
id: "SYSARCH-005"
type: system_architecture
name: "Single service binary and toolchain"
description: "One Go binary shipped as an OCI image, configured via its command line."
platform: "OCI container, linux/amd64 and linux/arm64"
satisfies:
  - "SYSREQ-022"
---

# System Architecture: Single service binary and toolchain

Normative: the service ships as one Go 1.26-or-later binary packaged as an OCI container image,
configured through its kong-parsed command line.

## Technical Strategy

One `main` package, `cmd/patchcaster`, wiring `internal/` packages: `ingest`, `store`, `deliver`,
`ops`, `config`. The `go.mod` `go` directive states the language minimum `1.26` with a pinned
toolchain line for reproducible builds; CGO stays off (pure-Go pgx driver, ADR-010) so the binary is
static. Images are built from a multi-stage Dockerfile to a distroless base. Common tasks —
`build`, `test`, `lint`, `migrate`, `container` — live in the `justfile` (ADR-007).

## Static View (Structure)

`cmd/patchcaster/main.go` parses configuration with kong (ADR-003), then composes the components
of SYSARCH-001/002/003/004 in explicit dependency order. No package-init magic: composition
happens in `main`, which is the only place global wiring is visible.

## Dynamic View (Behavior)

Start: config validation (SYSREQ-022) → logging → open store and apply goose migrations →
internal listener → public listener. Shutdown: the drain sequence of SYSREQ-023. Tests follow
testify with arrange-act-assert throughout (ADR-006); `go test ./...` is deterministic — no
network, no wall-clock dependence (injected clocks and `httptest` doubles).
