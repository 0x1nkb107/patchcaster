---
id: "SYSREQ-018"
type: system_requirement
name: "Health and readiness endpoints"
description: "Kubernetes probes reflect real serving capability."
specification: "The service SHALL expose a liveness endpoint and a readiness endpoint that reports failure while the state store cannot be written."
derives_from:
  - "SCEN-008"
---

# System Requirement: Health and readiness endpoints

## Requirement Specification

> The service SHALL expose a liveness endpoint and a readiness endpoint that reports failure while the state store cannot be written.

## Rationale

Kubernetes only guarantees what the probes tell it. Readiness must fail on conditions that
actually prevent serving (store unwritable, config missing); liveness must *not* fail on a
Mattermost outage, where restarting would only churn the pod while the queue is correctly backing off.

## Acceptance Criteria

- The liveness endpoint answers `200` whenever the process is serving at all, regardless of
  Mattermost state.
- The readiness endpoint answers `200` only when configuration is loaded and the state store
  accepts writes; it answers `503` otherwise.
- Both endpoints are served on the internal listener only (SYSARCH-004).

## Verification Plan

- **Method**: Test.
- **Procedure**: Endpoint tests with injected store-write failures and with the Mattermost mock
  in failing mode; assert probe answers in each combination.

## Notes

None.