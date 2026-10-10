---
id: "SYSREQ-031"
type: system_requirement
name: "Versioned release artifacts"
description: "A pushed version tag publishes a GitHub release and multi-architecture OCI images."
specification: "Pushing a semantic version tag to the repository SHALL publish that exact commit as a GitHub release and SHALL publish corresponding linux/amd64 and linux/arm64 OCI images under that version in the container registry."
derives_from:
  - "SCEN-010"
---

# System Requirement: Versioned release artifacts

## Requirement Specification

> Pushing a semantic version tag to the repository SHALL publish that exact commit as a GitHub
> release and SHALL publish corresponding linux/amd64 and linux/arm64 OCI images under that
> version in the container registry.

## Rationale

Rollouts and rollbacks (SCEN-010) are only safe if instances run identified, obtainable builds;
SYSARCH-005 claims both amd64 and arm64, so both must exist per release. Deciding the version at
release time also removes the `buildinfo`-at-ingest-time ambiguity from the binary. The pipeline
itself is the ADR-015 goreleaser run.

## Acceptance Criteria

- After a tag publish, the GitHub release lists the binaries for both architectures with a
  checksum file, all built from the tagged commit.
- The registry serves the version tag as a manifest list containing exactly
  `linux/amd64` and `linux/arm64`, and `latest` moves to it.
- Publishing is triggered only by tags: ordinary pushes build the same pipeline without
  making anything publicly visible.

## Verification Plan

- **Method**: Manual + configuration check.
- **Procedure**: `goreleaser check` validates the pipeline configuration in CI; end to end,
  push a `v*` tag to a staging clone and confirm the release listing and that both manifest
  digests serve. A snapshot run (`--skip=publish`) is the routine smoke test.

## Notes

The registry is GitHub Container Registry per ADR-015; artifact signing is out of scope for v1
(owner decision 2026-10-10).
