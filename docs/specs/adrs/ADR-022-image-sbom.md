---
id: "ADR-022"
type: architecture_decision_record
name: "Release images carry SBOM attestations"
description: "Every published image carries a BuildKit SBOM attestation per platform, covered by the image signature; the release archives ship no SBOM."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# ADR-022: Release images carry SBOM attestations

## Context

ADR-015 declared SBOMs out of scope for v1. goreleaser's `dockers_v2` step nonetheless defaults to `sbom: true` and passes `--attest=type=sbom` to `docker buildx build` when it pushes images, so released images would have carried an SBOM attestation that no decision accounted for. On 2026-10-12 the owner decided to ship SBOMs for the images, and not for the release archives.

## Decision

- Each published image carries an SBOM attestation for each platform, produced by BuildKit's scanner while the image is pushed. It lists the packages of the distroless base image and the Go modules built into the binary.
- The configuration sets `sbom` explicitly instead of relying on goreleaser's default, so the decision is visible where the images are defined.
- The attestations are attached to the image index. The cosign signature on the index digest (ADR-016) therefore covers them, because the index refers to each attestation by its digest.
- The images are built with a `docker-container` buildx builder, which supports attestations, index annotations and multi-platform pushes whatever image store the runner's Docker daemon uses.
- The release archives ship no SBOM. That would need syft as a new pinned tool, and its output carries timestamps and random document IDs that would break the reproducible checksums of ADR-021. (Amended on 2026-10-18 by ADR-044: the release also attaches an SBOM of the web interface's JavaScript packages, which PatchCaster's own build writes without timestamps, and which the signed checksums cover.)
- This amends the statement in ADR-015 that SBOMs are out of scope for v1. Homebrew taps stay out of scope.

## Consequences

- Good: operators can inspect what an image contains, from the registry, before they deploy it.
- Bad: snapshot builds do not attach attestations, so CI exercises the SBOM step for the first time at a real release. The `docker-container` builder keeps that step on the path goreleaser documents and tests. (Amended by ADR-023: the release rehearsal now pushes with attestations to a throwaway registry on every push to `main`.)
- The SBOM content follows BuildKit's scanner. It can change between releases even when the code does not, because the scanner image is not pinned.
