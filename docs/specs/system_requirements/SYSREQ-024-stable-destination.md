---
id: "SYSREQ-024"
type: system_requirement
name: "Immutable destination at enqueue"
description: "The routed channel is resolved at ingress and persisted with the queued message."
specification: "The service SHALL persist the resolved sink and its channel identifier alongside each queued message so that every retry uses the destination chosen at ingress."
derives_from:
  - "SCEN-005"
  - "SCEN-006"
depends_on:
  - "SYSREQ-017"
  - "SYSREQ-046"
---

# System Requirement: Immutable destination at enqueue

## Requirement Specification

> The service SHALL persist the resolved sink and its channel identifier alongside each queued message so that every retry uses the destination chosen at ingress.

## Rationale

Owner directive (2026-10-10). Routes change through the API while a queue drains; a change would
otherwise either redirect accepted messages or strand them when a route disappears. Persisting the resolved channel at enqueue makes the destination
part of the accepted fact — config history can move underneath the queue without changing a
single pending delivery.

## Acceptance Criteria

- Enqueue of a routed event persists its resolved channel identifier before the `2xx`
  acknowledgement (same row as payload and state, SYSREQ-017).
- Delivery retries after a route change post to the sink and channel stored on the row, not to any
  newly routed one.
- Removing a route with pending messages does not fail or redirect those messages; they carry
  identifiers, not a lookup.
- No code path resolves a destination at delivery or retry time.

## Verification Plan

- **Method**: Test.
- **Procedure**: Enqueue under one route; change the routes (route removed, sink added); drain
  the queue against the fake Mattermost; assert every recorded call targets the ingress-time
  channel.

## Notes

Complements SYSREQ-046: the routes are looked up at ingress; the sink and its channel identifier
are the immutable execution value. Amended on 2026-10-18 with the routes stored in the database. Config changes affecting only new events follows from this item —
no additional "config epoch" mechanism is needed.
