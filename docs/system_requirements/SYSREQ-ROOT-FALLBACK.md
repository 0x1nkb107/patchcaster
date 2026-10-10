---
id: "SYSREQ-007"
type: system_requirement
name: "Missing mapping fallback"
description: "Events for merge requests without a known root post start a new thread."
specification: "The service SHALL post a new root message when it forwards an event for a merge request whose root post mapping is unknown *(assumed)*."
derives_from:
  - "SCEN-002"
depends_on:
  - "SYSREQ-006"
---

# System Requirement: Missing mapping fallback

## Requirement Specification

> The service SHALL post a new root message when it forwards an event for a merge request whose root post mapping is unknown *(assumed)*.

## Rationale

Events legitimately arrive for merge requests the bot never announced — opened before the webhook
was installed, or after state loss. Replying to a lost root would be an API error, and dropping
the event would lose visibility silently; a fresh root is the self-healing choice *(assumed)*.

## Acceptance Criteria

- A forwarded event whose merge request has no stored mapping produces exactly one new root post
  in the routed channel, and the mapping is then stored.
- Subsequent events for that merge request extend this new root.

## Verification Plan

- **Method**: Test.
- **Procedure**: Empty store; deliver a `merge` event, assert a new root; then deliver `update`
  and assert it replies to that root.

## Notes

The follow-up root may note it is continuing an earlier conversation; wording is a design detail.
