---
id: "SYSREQ-033"
type: system_requirement
name: "Versioned Helm chart artifact"
description: "A pushed version tag also publishes the project's Helm chart as an OCI artifact whose appVersion matches the released image."
specification: "Pushing a semantic version tag to the charts repository SHALL publish a packaged Helm chart of PatchCaster as an OCI artifact at oci://ghcr.io/patchcaster-app/charts/patchcaster under that version, with chart version and appVersion both equal to the tag."
derives_from:
  - "SCEN-010"
depends_on:
  - "SYSREQ-031"
---

# System Requirement: Versioned Helm chart artifact

## Requirement Specification

> Pushing a semantic version tag to the repository SHALL publish a packaged Helm chart as an OCI artifact at `oci://ghcr.io/patchcaster-app/charts/patchcaster` under that version, with chart version and `appVersion` both equal to the tag.

## Rationale

The deployment target is Kubernetes (UC-003), and a chart is the installable unit there.
Version-follows-tag keeps the chart's `appVersion` truthful: every published chart names an
image that SYSREQ-031 published in the same run and SYSREQ-032 signed in the same run, so a
chart can never point at an image that does not exist or was cut from a different commit.
Chart scope (core workloads only, no ingress/HPA/bundled database) is design, decided in
ADR-017; the owner fixed the registry path, the versioning policy, and chart signing on
2026-10-10.

## Acceptance Criteria

- After a published release for tag `vX.Y.Z`, `helm pull oci://ghcr.io/patchcaster-app/charts/patchcaster --version X.Y.Z` succeeds.
- `helm show chart` of the pulled artifact reports `version: X.Y.Z` and `appVersion: X.Y.Z`.
- The pulled chart passes `helm lint --strict` unmodified.
- The chart artifact carries a cosign signature in the public transparency log (SYSREQ-032).

## Verification Plan

- **Method**: demonstration at the first published tag, plus inspection per release.
- **Procedure**: run the four acceptance commands against the published artifact; `cosign verify`
  with the release-workflow identity policy (README). Per-change rendering conformance is gated
  in CI by `just chart-check` (`helm lint --strict` + template render of the example values).

## Notes

Chart content (templates, values surface, scope) follows ADR-017; the chart lives in the sibling
`patchcaster-app/charts` repository and is tagged and published from there.