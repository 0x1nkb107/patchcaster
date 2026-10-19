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
an artifact the infrastructure team can pull or roll back to (SYSREQ-031). SYSARCH-005 claims
`linux/amd64` and `linux/arm64`, so publishing must cover both architectures, and the
single-replica operational posture makes reproducibility of the published artifact a
first-order concern, not a nicety.

## Decision

Release with goreleaser v2, pinned as a tools module exactly like the lint and codegen tools
(ADR-011, ADR-010): `go tool -modfile tools/goreleaser/go.mod goreleaser`, driven by
`just release-publish` (publish) and `just release-snapshot` (local dry run, never publishes).

- Builds are CGO-free static binaries for `linux/amd64` and `linux/arm64`, `-trimpath`,
  `-s -w`, with the version injected via `-X main.version` and surfaced by a `--version`
  flag, so a running container can state its own build.
- OCI images are built by `docker buildx` from a single COPY-only `Dockerfile`. With no `RUN`
  instructions the cross-architecture build pulls and copies without emulating any target
  binary, so no QEMU is required in CI. The same Dockerfile serves local work: `just
  container` compiles with the pinned host toolchain and stages the binary in the layout the
  Dockerfile reads, keeping podman (the project's engine) in the loop while no compiler ever
  runs inside an image.
- Images are published to the GitHub Container Registry (`ghcr.io`) under the version tag plus
  a `latest` floating tag tracking the most recent release; the GitHub release carries the
  archives and their checksums. SBOMs and Homebrew taps are explicitly out of scope
  for v1; artifact signing is ADR-016. (Amended by ADR-022: images now carry SBOM attestations, while the archives still ship none.)
- Publishing is triggered by pushing a `v*` tag in a dedicated GitHub Actions workflow using
  the workflow `GITHUB_TOKEN`; ordinary pushes run the same pipeline with `--snapshot` and
  publish nothing. (Amended by ADR-023: pushes to `main` now rehearse the full release procedure, still publishing nothing.)

## Consequences

- Cutting a release is `git tag vX.Y.Z && git push --tags`; no manual artifact steps remain.
  (Amended by ADR-018: the changelog is batched and committed first, and its version file
  becomes the release notes.)
- Published tags are immutable by convention: republishing would violate the promise of
  SYSREQ-031, so a bad cut means a new version, never a force-push.
- Toolchain coupling is real: goreleaser v2.18 requires Go 1.27, which motivated moving the
  project toolchain to the pinned 1.27.1 (owner-approved, 2026-10-10); the tools-module pin
  makes that coupling visible in `tools/goreleaser/go.mod` rather than ambient.
- The first image push to a private GitHub repository creates a private package; making the
  image pullable without login is a one-time registry setting, not a pipeline change.
- One Dockerfile serves both flows (owner decision, 2026-10-10, closing the pair drift
  concern): the image always wraps an externally compiled binary, so the in-container
  rebuild proof that the self-building dev image offered is retired; the pinned-toolchain
  guarantee moves fully onto host/runner builds, where `GOTOOLCHAIN=local` already enforces
  it.
