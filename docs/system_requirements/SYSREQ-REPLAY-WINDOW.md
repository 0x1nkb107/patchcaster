---
id: "SYSREQ-013"
type: system_requirement
name: "Freshness window"
description: "Stale deliveries are refused to bound replay risk."
specification: "The service SHALL reject with HTTP 401 deliveries whose webhook-timestamp is older than 300 seconds at arrival *(assumed)*."
derives_from:
  - "SCEN-004"
---

# System Requirement: Freshness window

## Requirement Specification

> The service SHALL reject with HTTP 401 deliveries whose webhook-timestamp is older than 300 seconds at arrival *(assumed)*.

## Rationale

The signature already covers the timestamp; bounding freshness turns a captured delivery into a
time-limited artifact. 300 seconds matches the Standard Webhooks SDK default and tolerates clock
skew plus GitLab retry pacing *(assumed)*. Future-dated timestamps beyond the window are refused
for the same bound-skew reason.

## Acceptance Criteria

- With a valid signature, a `webhook-timestamp` within ±300 s of service time proceeds; outside
  it answers `401` with zero side effects.
- The window is a single named constant or configuration value, referenced in one place.

## Verification Plan

- **Method**: Test.
- **Procedure**: Injected clock at boundary values (−301 s, −299 s, +301 s) with correctly signed
  fixtures.

## Notes

None.
