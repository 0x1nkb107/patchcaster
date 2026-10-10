---
id: "SYSREQ-017"
type: system_requirement
name: "Durable outbound queue"
description: "Messages pending delivery are persisted until accepted."
specification: "The service SHALL persist every outbound message until Mattermost accepts it and SHALL apply the delivery's derived merge request state in the same transaction that enqueues it, so that pending messages and state survive process termination together."
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

- The outbound message row is written before the `2xx` is sent for a routed event, together with
  the state changes the delivery implies (SYSREQ-027): one transaction admits, records, and
  applies — or admits nothing.
- The row persists the destination channel resolved at ingress; retries never re-resolve routing
  (SYSREQ-024).
- `SIGKILL` immediately after the acknowledgement leaves the message pending; the next process
  delivers it.
- A message whose acceptance was recorded is never re-posted. After a crash between Mattermost
  accepting a post and the queue recording that acceptance, the retry re-posts: the contract is
  at-least-once, with the duplicate risk confined to that window, because Mattermost post
  creation carries no client-supplied idempotency key.
- A routed event the store cannot durably accept is answered `5xx`, never `2xx` — the
  acknowledgement of SYSREQ-015 is issued only after the enqueue commit (GitLab then retries;
  readiness is already failing, SYSREQ-018).

## Verification Plan

- **Method**: Test.
- **Procedure**: Kill-and-restart integration test against a disposable PostgreSQL database; inspect row
  state after the kill and assert delivery after restart.

## Notes

Duplicate *deliveries from GitLab* are handled by SYSREQ-014; this item covers the service's own
delivery pipeline. The duplicate window between acceptance and record is accepted residual risk,
documented rather than eliminated: search-before-post as a mitigation was considered and rejected
as unbounded best-effort against an API that never promised idempotency.
