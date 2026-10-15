---
id: "ADR-024"
type: architecture_decision_record
name: "PatchCaster is licensed AGPL-3.0-only with a section 14 proxy"
description: "The project moves from MIT to the GNU Affero General Public License, version 3, plus later versions accepted by a proxy who is identified by a pseudonym and an SSH signing key."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# ADR-024: PatchCaster is licensed AGPL-3.0-only with a section 14 proxy

## Context

PatchCaster was licensed under MIT. On 2026-10-12 the owner decided to relicense it under the GNU Affero General Public License, version 3, with a proxy under section 14 of that licence who can accept later versions. The owner wants to be that proxy without publishing more personal data than the project already shows. No release had been published at that point, and the only commit authors were the owner and the owner's bot account, so no other contributor had to agree.

## Decision

- The licence expression is `AGPL-3.0-only OR LicenseRef-PatchCaster-Accepted-AGPL`. The first part is version 3 alone. The second is the same grant plus any later version that the proxy has accepted, as set out in `LICENSES/LicenseRef-PatchCaster-Accepted-AGPL.txt`.
- The proxy is named by the pseudonym 0x5a17ed and anchored to an SSH signing key, with fingerprint `SHA256:VkmVOLeXOaLEBGdFw/GhN6Bsm2FYkQ2bnnTmvwUXQQk`. An acceptance counts only if a statement signed with that key is published in the source repository. The key can be replaced, and a successor proxy can be named, by a statement signed the same way. No legal name and no contact data appear in the grant.
- The repository follows the REUSE layout. `LICENSES/` holds both licence texts, and `REUSE.toml` applies the expression to every file. The root `LICENSE` repeats the AGPL text, so that GitHub and Go tooling recognize the licence.
- Every distributed copy carries the licence texts, because the AGPL requires giving recipients a copy of the licence. The release archives carry `LICENSE` and the proxy grant next to the binary. The container image carries both files under `/usr/share/doc/patchcaster/`. The image label `org.opencontainers.image.licenses` carries the full expression.
- The copyright line stays "The patchcaster Authors".
- This amends ADR-020, which kept only `LICENSE` in the archives and gave the MIT licence as the reason.

## Consequences

- Good: the proxy can let the project follow future AGPL versions without asking every contributor, while the grant stays limited to versions the Free Software Foundation publishes.
- Good: a signature check, not a name or an account, decides whether a statement comes from the proxy. A renamed or lost GitHub account cannot change that.
- Bad: if the signing key is lost before a replacement is named, no later version can be accepted, and the project stays on version 3. That failure mode leaves the existing grant intact.
- Bad: under section 13 of the AGPL, anyone who runs a modified PatchCaster must offer its source to the users who interact with it over a network. How PatchCaster points those users to the source is still open.
- Everything linked into the binary is under MIT, BSD-3-Clause or Apache-2.0, which are compatible with the AGPL. New dependencies must stay compatible.
- Copies of the earlier history keep the MIT licence they were published under.
- The Helm chart in the separate charts repository keeps its own licence until that repository decides separately.
