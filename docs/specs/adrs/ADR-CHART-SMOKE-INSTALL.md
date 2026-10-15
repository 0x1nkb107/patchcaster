---
id: "ADR-028"
type: architecture_decision_record
name: "The chart pulls the application's release tag and installs it before it publishes"
description: "The Helm chart defaults its image to v<appVersion>, CI installs the chart for real as a pinned application release, and the chart release installs itself as its own tag before it pushes anything."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-005"
---

# ADR-028: The chart pulls the application's release tag and installs it before it publishes

## Context

ADR-017 gated chart changes with lint, a render and a server-side dry run, because the application image might still be private. Live installs were to follow the first application release. v0.1.0 was released on 2026-10-13, and its image became public the same day. A first live install of the chart, packaged the way its release packages it, never turned ready. Chart releases set `appVersion` to the bare `X.Y.Z`, as SYSREQ-033 requires, and the image tag defaulted to `appVersion`. The application tags its images `vX.Y.Z`, so the chart pulled a tag that does not exist. The charts repository was private until the same day, and its chart had no values for five of the application's settings.

## Decision

- Without an explicit tag the chart pulls `v<appVersion>`, the application's release tag. `appVersion` stays the bare version of SYSREQ-033, and a leading `v` in it is not doubled. An explicit tag or digest still wins.
- `just chart-smoke <app-version>` in the charts repository packages the chart as that release and installs it into a throwaway kind cluster next to a disposable PostgreSQL. It passes when the pod turns ready. The cluster gets its own kubeconfig file, so a developer's kubectl configuration stays untouched.
- CI runs the smoke install for every chart change, pinned to the latest application release. The pin moves by hand with each application release.
- The chart's release workflow runs the smoke install for its own tag before it pushes. A chart tag with no application image of that version, or with an image that does not start, publishes nothing.
- The chart has a value for every `serve` setting of the application. An empty tuning value keeps the application's default, so the chart repeats no default that could drift.
- The charts repository stays MIT-licensed, unlike the application of ADR-024, and declares the licence in its chart metadata. The repository and both packages are public.

## Consequences

- Good: a published chart installs and starts the application release it names. The release workflow, not acceptance after the fact, catches a chart that names a missing image.
- Good: the smoke install also proves that the application accepts the environment the chart renders, because its values set every tuning value.
- Bad: each application release needs a second change in the charts repository to move the CI pin. Until then, CI keeps testing the previous release.
- Bad: the smoke install adds a few minutes to every chart change and release, and it pulls PostgreSQL from Docker Hub.
