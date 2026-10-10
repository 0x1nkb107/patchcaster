---
id: "SYSARCH-005"
type: system_architecture
name: "Single service binary and toolchain"
description: "One Go binary shipped as an OCI image, configured via its command line."
platform: "OCI container, linux/amd64 and linux/arm64"
satisfies:
  - "SYSREQ-022"
  - "SYSREQ-031"
  - "SYSREQ-032"
  - "SYSREQ-033"
---

# System Architecture: Single service binary and toolchain

Normative: the service ships as one Go 1.26-or-later binary packaged as an OCI container image,
configured through its kong-parsed command line.

## Technical Strategy

One `main` package, `cmd/patchcaster`, wiring `internal/` packages: `ingest`, `store`, `deliver`,
`ops`. The kong-parsed configuration structs live in the `main` package itself — the command line
is the command's own surface, parsed and validated there (owner directive 2026-10-10). The
`go.mod` `go` directive pins the exact toolchain (currently
`1.27.1`) for reproducible builds; CGO stays off (pure-Go pgx driver, ADR-010) so the binary is
static. Images are assembled by one COPY-only distroless Dockerfile around binaries compiled by
that pinned toolchain — release images via buildx multi-arch (ADR-015), local images by
`just container`. Common tasks —
`build`, `test`, `lint`, `migrate`, `container`, `devdb` — live in the `justfile` (ADR-007),
with `lint` running the version-pinned `golangci-lint` gate (ADR-011). Release artifacts are cut
by the version-pinned goreleaser pipeline of ADR-015 (`just release`), which cross-builds both
claimed architectures and publishes the images and GitHub release of SYSREQ-031, signing images
and archives keylessly per ADR-016 (SYSREQ-032). The Helm chart of SYSREQ-033 ships out-of-band
from the sibling charts repository, signed under that repository's own workflow identity
(ADR-017).

## Static View (Structure)

`cmd/patchcaster/main.go` parses configuration with kong (ADR-003), then composes the components
of SYSARCH-001/002/003/004 in explicit dependency order. No package-init magic: composition
happens in `main`, which is the only place global wiring is visible.

## Dynamic View (Behavior)

Start: config validation (SYSREQ-022) → logging → open store and apply goose migrations →
internal listener → public listener. Shutdown: the drain sequence of SYSREQ-023. Tests follow
testify with arrange-act-assert throughout (ADR-006); `go test ./...` is deterministic — no
network, no wall-clock dependence (injected clocks and `httptest` doubles).