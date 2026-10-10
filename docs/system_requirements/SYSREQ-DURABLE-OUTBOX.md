---
id: "SYSREQ-017"
type: system_requirement
name: "Durable outbound queue"
description: "Messages pending delivery are persisted until accepted."
specification: "The service SHALL persist every outbound message until Mattermost accepts it so that messages pending at process termination are delivered after restart."
derives_from:
  - "SCEN-005"
depends_on:
  - "SYSREQ-016"
---

# System Requirement: Durable outbound queue

## Requirement Specification

> The service SHALL persist every outbound message until Mattermost accepts it so that messages pending at process termination are delivered after restart.

## Rationale

An in-memory queue turns the honest `2xx` of SYSREQ-015 into a broken promise on crash. Writing
the message before acknowledging makes acceptance a durable commitment: the system is at-least-once
end to end.

## Acceptance Criteria

- The outbound message row is written before the `2xx` is sent for a routed event.
- `SIGKILL` immediately after the acknowledgement leaves the message pending; the next process
  delivers it.
- Re-claimed messages are delivered exactly once from the service's side (a post accepted by
  Mattermost is never re-posted).

## Verification Plan

- **Method**: Test.
- **Procedure**: Kill-and-restart integration test over a temporary database file; inspect row
  state after the kill and assert delivery after restart.

## Notes

Duplicate *deliveries from GitLab* are handled by SYSREQ-014; this item covers the service's own
delivery pipeline.
