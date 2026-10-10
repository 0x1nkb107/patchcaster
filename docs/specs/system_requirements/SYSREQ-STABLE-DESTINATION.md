---
id: "SYSREQ-024"
type: system_requirement
name: "Immutable destination at enqueue"
description: "The routed channel is resolved at ingress and persisted with the queued message."
specification: "The service SHALL persist the resolved Mattermost channel identifier alongside each queued message so that every retry uses the destination chosen at ingress."
derives_from:
  - "SCEN-005"
  - "SCEN-006"
depends_on:
  - "SYSREQ-009"
  - "SYSREQ-017"
---

# System Requirement: Immutable destination at enqueue

## Requirement Specification

> The service SHALL persist the resolved Mattermost channel identifier alongside each queued message so that every retry uses the destination chosen at ingress.

## Rationale

Owner directive (2026-10-10). The routing table is deployed through GitOps; a rollout that
changes routes while a queue drains would otherwise either redirect accepted messages or strand
them when an entry disappears. Persisting the resolved channel at enqueue makes the destination
part of the accepted fact — config history can move underneath the queue without changing a
single pending delivery.

## Acceptance Criteria

- Enqueue of a routed event persists its resolved channel identifier before the `2xx`
  acknowledgement (same row as payload and state, SYSREQ-017).
- Delivery retries after a routing-table change post to the channel stored on the row, not to any
  newly configured channel.
- Removing the routing entry of a project with pending messages does not fail or redirect those
  messages; they carry an identifier, not a path.
- No code path resolves a destination at delivery or retry time.

## Verification Plan

- **Method**: Test.
- **Procedure**: Enqueue under routing table A; replace the table (entry removed, channel
  swapped); drain the queue against the fake Mattermost; assert every recorded call targets the
  ingress-time channel.

## Notes

Complements SYSREQ-009: the path is the lookup key at ingress; the channel identifier is the
immutable execution value. Config changes affecting only new events follows from this item —
no additional "config epoch" mechanism is needed.
