---
id: "SYSREQ-005"
type: system_requirement
name: "Threaded replies for later activity"
description: "Later events extend the merge request thread instead of posting new roots."
specification: "The service SHALL post every forwarded event for an already announced merge request as a reply within the existing thread of that merge request."
derives_from:
  - "SCEN-002"
depends_on:
  - "SYSREQ-006"
---

# System Requirement: Threaded replies for later activity

## Requirement Specification

> The service SHALL post every forwarded event for an already announced merge request as a reply within the existing thread of that merge request.

## Rationale

This is the product decision in one sentence: conversation, not firehose. `merge` and `close` are
later activity on an announced merge request, so they extend the thread rather than starting a
new message (owner intent, 2026-10-10).

## Acceptance Criteria

- For a merge request with a known mapping, each accepted `update`, `approval`, `approved`,
  `merge`, or `close` delivery causes exactly one post whose `root_id` is the stored root post
  identifier, in the root's channel.
- Each reply names the acting user and the action so the thread reads coherently.
- A service restart between root and reply does not change the target thread (SYSREQ-006).

## Verification Plan

- **Method**: Test.
- **Procedure**: Script the event sequence `open`, `update`, `approval`, `approved`, `merge`
  against the fake API; assert one root and one reply per event, all in one thread.

## Notes

None.
