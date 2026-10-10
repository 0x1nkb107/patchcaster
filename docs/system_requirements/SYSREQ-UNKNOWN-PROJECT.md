---
id: "SYSREQ-010"
type: system_requirement
name: "Unconfigured projects acknowledge quietly"
description: "Deliveries for unconfigured projects are acknowledged without posting."
specification: "The service SHALL answer deliveries for projects absent from the routing configuration with HTTP 2xx without posting to Mattermost."
derives_from:
  - "SCEN-003"
---

# System Requirement: Unconfigured projects acknowledge quietly

## Requirement Specification

> The service SHALL answer deliveries for projects absent from the routing configuration with HTTP 2xx without posting to Mattermost.

## Rationale

With a shared system hook the instance delivers everything. Answering `4xx` would make GitLab
retry and eventually auto-disable the hook (failure counting is instance-wide), harming every
configured project; silent `2xx` keeps the hook healthy and the channels clean.

## Acceptance Criteria

- A verified delivery whose project path has no routing entry answers `2xx`.
- Zero Mattermost request attempts are made for it, one structured log line names the project and
  `webhook-id`, and the ignored-delivery counter increments (SYSREQ-019).

## Verification Plan

- **Method**: Test.
- **Procedure**: Handler test with a recording Mattermost mock asserting no outbound calls.

## Notes

None.
