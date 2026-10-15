---
id: "SYSREQ-002"
type: system_requirement
name: "Root post per new merge request"
description: "Opening a merge request starts a new thread in the mapped channel."
specification: "The service SHALL post a new root message to the mapped Mattermost channel when it accepts a forwarded merge request event whose action is open."
derives_from:
  - "SCEN-001"
depends_on:
  - "SYSREQ-001"
---

# System Requirement: Root post per new merge request

## Requirement Specification

> The service SHALL post a new root message to the mapped Mattermost channel when it accepts a forwarded merge request event whose action is open.

## Rationale

The root post is the anchor of the merge request's thread; creating it exactly once and storing
its identifier before any other event is handled is what makes the thread stable.

## Acceptance Criteria

- An accepted `open` event causes exactly one Mattermost post carrying no `root_id`, addressed to
  the routed channel.
- The returned post identifier is persisted before any other event for that merge request can
  produce a post (SYSREQ-006).
- A suppressed duplicate `open` delivery (SYSREQ-014) does not create a second root.

## Verification Plan

- **Method**: Test.
- **Procedure**: Recording fake of the Mattermost posts API; replay `open`, duplicate `open`,
  assert exactly one post and a stored mapping.

## Notes

Delivery mechanics live in SYSARCH-003; this item fixes the observable behaviour. The
exactly-one guarantee is subject to the crash window stated in SYSREQ-017's notes: duplicate risk
exists only for a crash between Mattermost acceptance and the queue recording it.
