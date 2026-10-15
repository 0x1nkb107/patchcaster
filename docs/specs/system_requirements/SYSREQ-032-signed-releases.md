---
id: "SYSREQ-032"
type: system_requirement
name: "Signed release provenance"
description: "Published images and archives carry cosign signatures verifiable against the release workflow identity."
specification: "Published release images, release archives, and Helm chart artifacts SHALL carry cosign signatures recorded in a public transparency log and verifiable against the project's release workflow identity without pre-distributed keys."
derives_from:
  - "SCEN-010"
---

# System Requirement: Signed release provenance

## Requirement Specification

> Published release images and release archives SHALL carry cosign signatures recorded in a
> public transparency log and verifiable against the project's release workflow identity
> without pre-distributed keys.

## Rationale

Rollouts (SCEN-010) trust that the pulled artifact came from this repository's pipeline;
SYSREQ-031 provides obtainability, this requirement provides authenticity. Keyless signing
bound to the workflow identity (ADR-016) removes secret management and gives Kubernetes the
identity-based verification it natively supports.

## Acceptance Criteria

- `cosign verify` on a published version tag succeeds when pinned to the release workflow's
  certificate identity and GitHub's OIDC issuer, and fails for unsigned or foreign images.
- The GitHub release carries a signature bundle for the checksum file; `cosign verify-blob
  --bundle` proves the published archives match the signed digests.
- A signing failure fails the release job: no artifact path produces unsigned output on a
  green pipeline. (goreleaser publishes before signing, so a failed sign leaves a release to
  be retried or discarded — never a green run over unsigned artifacts.)

## Verification Plan

- **Method**: Manual + pipeline.
- **Procedure**: After a tag push, run the verification commands in `docs/install.md` against the image and
  the archive bundle; the CI release rehearsal (ADR-023) proves that runs without a release identity skip signing cleanly
  (`--skip=sign`) instead of attempting identities they cannot obtain.

## Notes

Keyless mode per ADR-016; the verifier-side identity pinning is normative documentation, kept
in `docs/install.md` next to the commands.