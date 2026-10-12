---
id: "ADR-023"
type: architecture_decision_record
name: "Pushes to main rehearse the release without publishing"
description: "Every push to main runs the documented release procedure inside the CI runner, pushes images to a throwaway registry and checks the release contract, without publishing anything."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# ADR-023: Pushes to main rehearse the release without publishing

## Context

The Release workflow's smoke job ran `goreleaser release --snapshot`. A snapshot skips validation and publishing, builds one image per platform instead of the multi-platform index, and attaches no SBOM attestation. The changelog batch, the release tag, the `release-publish` recipe and the image push therefore ran for the first time at a real release. The owner wants CI to build trust that tagging a release works, without producing anything: on 2026-10-12 they turned down publishing images from `main` and asked for a test of the release pipeline that releases nothing. They then chose to rehearse pushes to `main` only, because `main` is what gets tagged.

## Decision

- Every push to `main` runs a release rehearsal instead of the snapshot smoke job. Manual runs of the workflow rehearse too. Inside the runner's checkout it adds a placeholder changelog fragment, runs `just release-changelog`, commits the batch, runs `just release-tag` and then `just release-publish --skip=sign`. That is the documented release procedure, with real validation, a real version from the tag and the real image push.
- Nothing leaves the runner. The commit and the tag are never pushed. The images go to a registry container that exists only for the job. The GitHub release is switched off, and signing is skipped.
- Two environment switches carry the difference, and real releases leave both unset. `PATCHCASTER_IMAGE_REPO` names the image repository and defaults to the GitHub registry. `PATCHCASTER_RELEASE_REHEARSAL` switches the GitHub release off.
- A check script then asserts the release contract against the registry and the build output: an index with exactly `linux/amd64` and `linux/arm64`, `latest` on the same digest, an SBOM attestation per platform, the OCI annotations and labels, the archive contents, the checksums and the version the binary reports.
- Signing stays skipped, because keyless signing needs the identity of a tag run and writes a public entry to the transparency log.
- Pull requests run no rehearsal. They still validate the release configuration with `goreleaser check`, so a broken configuration cannot merge.
- `just release-snapshot` stays as the local dry run.
- This amends ADR-015, which had ordinary pushes run `--snapshot`, and ADR-022, which expected the SBOM step to run first at a real release.

## Consequences

- Good: every push to `main` proves, on the very commit that would be tagged, the changelog batch, the tag, the version derivation, the clean-tree and tag checks, the archives and checksums, and the multi-platform push with attestations and annotations.
- Bad: a release procedure that breaks shows up only after merge, on `main`, though still before anyone tags.
- Bad: keyless signing, the login and push to the GitHub registry, creating the GitHub release with its assets, and the tag trigger itself still run only at a real release.
- The rehearsal's buildx builder uses host networking so that BuildKit can reach the registry on `localhost`. That is the only difference from the release job's builder.
- The placeholder fragment means a rehearsal works right after a release too, when no fragments are pending, but its version is one step past what the pending fragments alone would give.
