---
id: "ADR-021"
type: architecture_decision_record
name: "Release archives are reproducible from their tag"
description: "Builds and archives take every timestamp from the tagged commit and pin file ownership and modes, so the same tag yields byte-identical archives on any machine."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# ADR-021: Release archives are reproducible from their tag

## Context

Two goreleaser builds of the same commit produced different archives and checksums. The binary carried the time of the build, and the archive recorded each file's modification time, owner, group and mode as found on the building machine: the checkout time of `LICENSE`, the builder's user and group IDs, and a mode that followed the builder's umask. Nobody could rebuild a release and compare it with the published checksums. On 2026-10-12 the owner decided that release artifacts become reproducible.

## Decision

- The build sets `mod_timestamp` to the commit timestamp, so the binary carries the commit's time. `-trimpath` already keeps local paths out of it.
- The archive pins every entry's metadata: owner and group `root`, mode `0755` for the binary and `0644` for `LICENSE`, and the commit date as modification time.
- Any file added to the archives later needs the same pinned metadata.
- (Amended on 2026-10-18 by ADR-032 and ADR-044: the binary embeds the web interface, whose bundle and SBOM are reproducible too. SvelteKit names the build after the commit, `web/.nvmrc` names the exact Node version, and the release rehearsal builds the bundle twice and compares the bytes.)

## Consequences

- Good: the same tag yields byte-identical archives wherever it is built, so anyone can rebuild a release and compare it with the signed checksums of ADR-016.
- Good: snapshot builds of one commit can be compared directly, which makes changes to the pipeline easier to review.
- Bad: the binary is identical only when it is built with the same Go toolchain and module versions. The toolchain pin of ADR-015 provides that, but a builder who overrides it gets a different binary.
- The container images are not covered by this decision. Their layers depend on the base image and on the build tool.
