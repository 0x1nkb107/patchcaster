---
id: "ADR-015"
type: architecture_decision_record
name: "goreleaser cuts releases"
description: "Version tags publish GitHub releases and multi-architecture OCI images through a version-pinned goreleaser run."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# ADR-015: goreleaser cuts releases

## Context

`just container` produces throwaway images built from the working tree; nothing turns a tag into
an artifact an operator can pull or roll back to (SYSREQ-031). SYSARCH-005 claims
`linux/amd64` and `linux/arm64`, so publishing must cover both architectures, and the
single-replica operational posture makes reproducibility of the published artifact a
first-order concern, not a nicety.

## Decision

Release with goreleaser v2, pinned as a tools module exactly like the lint and codegen tools
(ADR-011, ADR-010): `go tool -modfile tools/goreleaser/go.mod goreleaser`, driven by
`just release` (publish) and `just release-snapshot` (local dry run, never publishes).

- Builds are CGO-free static binaries for `linux/amd64` and `linux/arm64`, `-trimpath`,
  `-s -w`, with the version injected via `-X main.version` and surfaced by a `--version`
  flag, so a running container can state its own build.
- OCI images are built by `docker buildx` (docker-container driver) from a COPY-only
  `Dockerfile.goreleaser` sharing the distroless base of the dev Dockerfile. With no `RUN`
  instructions the cross-architecture build pulls and copies without emulating any target
  binary, so no QEMU is required in CI.
- Images are published to the GitHub Container Registry (`ghcr.io`) under the version tag plus
  a `latest` floating tag tracking the most recent release; the GitHub release carries the
  archives and their checksums. SBOMs and Homebrew taps are explicitly out of scope
  for v1; artifact signing is ADR-016.
- Publishing is triggered by pushing a `v*` tag in a dedicated GitHub Actions workflow using
  the workflow `GITHUB_TOKEN`; ordinary pushes run the same pipeline with `--snapshot` and
  publish nothing.

## Consequences

- Cutting a release is `git tag vX.Y.Z && git push --tags`; no manual artifact steps remain.
- Published tags are immutable by convention: republishing would violate the promise of
  SYSREQ-031, so a bad cut means a new version, never a force-push.
- Toolchain coupling is real: goreleaser v2.18 requires Go 1.27, which motivated moving the
  project toolchain to the pinned 1.27.1 (owner-approved, 2026-10-10); the tools-module pin
  makes that coupling visible in `tools/goreleaser/go.mod` rather than ambient.
- The first image push to a private GitHub repository creates a private package; making the
  image pullable without login is a one-time registry setting, not a pipeline change.
- `Dockerfile.goreleaser` and `Dockerfile` now share the distroless stage and can drift; the
  snapshot smoke run of the release workflow is the drift alarm.
