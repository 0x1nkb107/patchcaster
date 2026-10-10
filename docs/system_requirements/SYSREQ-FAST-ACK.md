---
id: "SYSREQ-015"
type: system_requirement
name: "Fast acknowledgement"
description: "GitLab receives a quick response before delivery is attempted."
specification: "The service SHALL answer an accepted delivery with HTTP 2xx within 1000 milliseconds at the 95th percentile on the reference host without waiting for the Mattermost post to complete."
derives_from:
  - "SCEN-005"
---

# System Requirement: Fast acknowledgement

## Requirement Specification

> The service SHALL answer an accepted delivery with HTTP 2xx within 1000 milliseconds at the 95th percentile on the reference host without waiting for the Mattermost post to complete.

## Rationale

GitLab disables webhooks after enough consecutive failures or timeouts; slow or delivery-coupled
acknowledgements put the whole feed at risk. Decoupling acceptance from delivery also makes the
outage behaviour of SCEN-005 possible. The 1000 ms target is an internal budget well inside
GitLab's timeout; budget and reference host are owner-approved (2026-10-10).

## Acceptance Criteria

- The `2xx` response is written before any Mattermost request is attempted for that event, and
  only after the accepted event is durably enqueued (SYSREQ-017): the acknowledgement is a
  promise, not a receipt (owner condition, 2026-10-10).
- At 50 deliveries/s steady state on the reference host (2 vCPU, 2 GB, containerized), response
  p95 stays at or under 1000 ms even when Mattermost answers slowly.

## Verification Plan

- **Method**: Test + benchmark.
- **Procedure**: Handler benchmark against a fake Mattermost with multi-second latency; assert
  the response-time histogram independently of delivery completion.

## Notes

The reference host definition is normative; both figures were owner-approved (2026-10-10) and
can be tightened later on field data by amending this item.