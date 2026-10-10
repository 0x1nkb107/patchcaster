---
id: "ADR-016"
type: architecture_decision_record
name: "Keyless cosign signing of release artifacts"
description: "Release images and archives are cosign-signed via GitHub Actions OIDC identity, with signatures in the public transparency log."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# ADR-016: Keyless cosign signing of release artifacts

## Context

SYSREQ-031 made tagged builds obtainable, but obtainability is not authenticity: the rollouts
and rollbacks of SCEN-010 assume the image pulled from the registry was produced by this
repository's pipeline. A signing key in a repository secret reintroduces exactly the failure
mode signing should remove — the key can be exfiltrated, must be rotated, and proves only that
a key holder signed, not which workload. GitHub Actions already issues short-lived OIDC
identities, and Sigstore's Fulcio/Rekor turn that identity into keys nobody has to keep. The
deployment target is Kubernetes, where verification against workload identity (Policy
Controller `ClusterImagePolicy`) is the native pattern.

## Decision

Sign releases with cosign in **keyless mode**, wired into the goreleaser pipeline of ADR-015:

- cosign v3 is installed in the release job by the `sigstore/cosign-installer` action, pinned
  by commit SHA and given an explicit `cosign-release` version. The installer checks the
  downloaded binary against the cosign release key before putting it on `PATH`, where
  goreleaser finds it. cosign was first a tools module (`tools/cosign`) like every other
  tool; the owner replaced it with the action on 2026-10-10, as cosign only ever runs in that
  job.
- The release archives' `checksums.txt` gets a cosign *bundle* (signature + certificate + log
  entry) attached to the GitHub release; the OCI manifest lists get `cosign sign` signatures
  in the registry itself. Both cover the `latest` tag.
- Fulcio/Rekor with the default public transparency log; no CA or log infrastructure is run.
- Trust anchors on verification, not on a key: verifiers pin the certificate identity to this
  repository's release workflow and a `v*` tag, and the issuer to GitHub's OIDC provider. The
  exact commands are documented for operators (`docs/install.md`).
- Snapshots and dry runs skip signing (`--skip=sign`): keyless has no identity to ask outside
  Actions, and a snapshot that "signed" would be a lie.
- If releases ever need to run outside GitHub Actions, the explicit fallback is a KMS-backed
  keyless replacement (Fulcio supports it), not a long-lived private key.

## Consequences

- Publishing gains no new secrets: the workflow's own OIDC token is the credential.
- The trust domain becomes the repository's default branch: whoever can merge a change to
  the release workflow can mint a "valid" signature, the same domain that already controls
  publishing. Verification pinning makes that boundary visible and reviewable.
- Signing failure fails the release job after publication (goreleaser signs after publishing):
  a partially-signed release is retried or discarded, never rolled out — stated as a failure
  rule in SYSREQ-032.
- Rekor entries for a public repository disclose tag timing to the public log; the releases
  are public already, so nothing new leaks.
- Verification is now an operator-facing contract; breaking the workflow path (renaming the
  file, changing the tag scheme) silently breaks every verifier, so those commands double as
  canaries in the release acceptance.
