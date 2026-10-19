---
id: "ADR-020"
type: architecture_decision_record
name: "Release archives carry only the binary and its licence"
description: "Each release archive holds the patchcaster binary and LICENSE; the README, the changelog and the deployment docs are no longer packaged."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# ADR-020: Release archives carry only the binary and its licence

## Context

The release archives of ADR-015 shipped `README.md`, `CHANGELOG.md` and the deployment docs next to the binary and `LICENSE`, laid out so that the README's links into `docs/` resolved inside an unpacked archive. ADR-018 recorded the changelog's place in them. On 2026-10-11 the owner decided that the archives do not need these files.

## Decision

- Each archive contains the `patchcaster` binary and `LICENSE`, nothing else. `LICENSE` stays because the MIT licence requires its notice in every copy of the software. (Amended by ADR-024: the project is now under the AGPL, which requires a copy of the licence in every copy, and the archives also carry the proxy grant.)
- `README.md`, `CHANGELOG.md` and the deployment docs are no longer packaged. The release notes remain the body of the GitHub release, built from the same changie batch as `CHANGELOG.md` (ADR-018).
- This amends the distribution rule of ADR-018. The archive names, platforms, checksums and signatures of ADR-015 and ADR-016 stay as they are.

## Consequences

- Good: an archive holds what runs, and the documentation has one home, the repository, instead of copies in every archive.
- Bad: someone who unpacks an archive on a machine without network access gets no documentation with it. The docs for a release are still in the source archive that GitHub attaches to every release.
- The archive contents were never part of the compatibility contract, which covers archive names, platforms and signing identities, so the change needs no Breaking entry in the changelog.
