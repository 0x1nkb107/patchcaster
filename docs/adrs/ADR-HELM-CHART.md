---
id: "ADR-017"
type: architecture_decision_record
name: "Ship a first-party Helm chart from a sibling charts repository"
description: "The project's Helm chart lives in a sibling repository, published as an OCI artifact to ghcr.io and signed under that repository's own keyless identity."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# ADR-017: Ship a first-party Helm chart from a sibling charts repository

## Context

The deployment target is Kubernetes (UC-003), where the chart is the installable unit. No graph
item ever forbade charts — an early claim that ADR-010 excluded them was checked against the
graph and found wrong (ADR-010 is PostgreSQL); the "out of scope" posture was conversational
only. On 2026-10-10 the owner decided to ship a first-party chart and to host it **outside** the
application repository, as `patchcaster-app/patchcaster-charts` alongside the many sibling
repositories of the workspace.

## Decision

- **Location & identity**: chart source lives in the sibling repository under `charts/patchcaster/`;
  the OCI distribution point is `oci://ghcr.io/patchcaster-app/charts`, so the chart resolves as
  `oci://ghcr.io/patchcaster-app/charts/patchcaster:<version>` — a separate package from the
  image, with separate visibility controls.
- **Trigger & versioning**: the charts repository releases from **its own** `v*` tags via its own
  Actions workflow. Chart `version` and `appVersion` both equal the tag (SYSREQ-033); parity with
  an application release is a release-time convention verified at acceptance, *not* enforced by
  cross-repo automation: a tag-triggered dispatch would need a cross-repo token, and the
  organization deliberately carries no repository secrets (ADR-016).
- **Publishing mechanism**: version-pinned `helm.sh/helm/v3` (tools module, v3.22.0) running
  `helm package --version <tag> --app-version <tag>` followed by `helm push … oci://` — the
  canonical OCI path from the Helm registries documentation. The chart repository is not a
  goreleaser consumer; ADR-015 stays scoped to the application.
- **Signing**: the chart artifact is cosign-signed keyless (ADR-016 mechanism) against its push
  digest. The certificate identity is the **charts** repository's workflow —
  `github.com/patchcaster-app/patchcaster-charts/.github/workflows/release.yml@refs/tags/…` —
  so verifiers must accept the union of the application and charts release identities.
- **Scope (core only)**: Deployment (liveness/readiness on the internal listener; hardened
  securityContext with `readOnlyRootFilesystem`; `terminationGracePeriodSeconds` derived to be
  strictly greater than `PATCHCASTER_DRAIN_TIMEOUT`, the SYSREQ-023 owner condition), Service
  (webhook port plus the internal port for in-cluster metrics scraping), optional
  ServiceAccount, Secret (or `existingSecret`), a routing ConfigMap as raw pass-through
  (SYSREQ-022), and resources/scheduling knobs. Explicitly **not** rendered: Ingress, HPA
  (contradicts the single-replica posture), and any bundled PostgreSQL (external by ADR-010).
- **CI gate**: every chart change runs `helm lint --strict` and a `helm template` render of the
  example values, mirroring the quality-gate posture of the application repository.

## Consequences

- Good: `helm install … oci://…` with verifiable provenance; the application repository's
  release pipeline stays a single-binary-plus-image concern.
- Bad: two repositories and two tag cuts per coordinated release; version drift is possible and
  is caught by inspection (SYSREQ-033 acceptance commands), not prevented.
- Bad: two cosign identity strings exist; verification examples in both READMEs must carry both.
- Bad: a chart version may name an `appVersion` whose image was never published; release
  acceptance (pull + `helm show chart`) catches it, the pipeline cannot.

## Alternatives Considered

- **In-repo `chart/`** published from the application workflow: one tag cuts everything, but the
  owner chose repository separation and independent chart iteration.
- **goreleaser `helm` stage**: the OSS helm publisher is oriented at git-hosted chart
  repositories; OCI push via `helm push` is the documented canonical path.
- **Cross-repo dispatch from the app tag**: enforces parity but requires a PAT — excluded by the
  no-secrets posture (ADR-016).
- **HTTP chart repository with `index.yaml`**: rejected; OCI is native since Helm 3.8 and avoids
  maintaining an index and its commits.
