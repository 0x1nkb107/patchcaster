---
id: "SYSREQ-023"
type: system_requirement
name: "Graceful drain on shutdown"
description: "Shutdown finishes in-flight work within a bounded window."
specification: "The service SHALL stop accepting new deliveries upon SIGTERM and finish in-flight deliveries within 30 seconds before exiting."
derives_from:
  - "SCEN-010"
---

# System Requirement: Graceful drain on shutdown

## Requirement Specification

> The service SHALL stop accepting new deliveries upon SIGTERM and finish in-flight deliveries within 30 seconds before exiting.

## Rationale

The outbox (SYSREQ-017) guarantees nothing is lost from disk; draining guarantees the work
already in hands also completes, so rollouts are event-invisible. 30 s fits a standard
`terminationGracePeriodSeconds` and bounds rollout time; owner-approved (2026-10-10) on the
condition that the deployment grants a termination grace period of at least this budget.

## Acceptance Criteria

- On `SIGTERM` the public listener immediately stops accepting new deliveries (new connections
  are refused or answered `503`).
- Workers stop claiming new messages and finish the attempt in progress; whatever remains stays
  pending in the store.
- The process exits with status `0` once drained, or force-stops at the 30-second budget.

## Verification Plan

- **Method**: Test.
- **Procedure**: Send `SIGTERM` to the running process mid-delivery against a slow Mattermost
  mock; assert completion, exit code, and pending-row state. The end-to-end suite implements it as `TestSIGTERMLetsTheDeliveryInHandFinish` (ADR-025).

## Notes

Owner condition (2026-10-10): deployment manifests set `terminationGracePeriodSeconds` strictly
above this 30 s budget; readiness flips to `503` at signal time (SYSARCH-004) so the Service
detaches the pod while the drain runs (UC-003).