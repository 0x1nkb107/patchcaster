---
id: "SCEN-002"
type: scenario
name: "Ongoing activity extends the thread"
description: "Updates, approvals, merges and closes for that merge request appear as replies in the same thread."
refines:
  - "UC-001"
---

# Scenario: Ongoing activity extends the thread

## Overview

After the root post exists, every allowlisted event for the same merge request appears as a reply
inside that thread — including across service restarts.

## Initial State

- The merge request was announced earlier (root post exists; mapping is stored).

## Trigger

GitLab delivers an action of `update`, `approval`, `approved`, `merge`, or `close` for that merge request.

## Step-by-Step Flow

1. Verification, deduplication, allowlist, and routing proceed as in SCEN-001.
2. The delivery worker looks up the stored root post identifier.
3. It posts the reply with that `root_id` into the mapped channel.

## Expected Outcome

- One reply per accepted delivery, always inside the same thread and channel as the root post.
- A restart between root and reply does not split the conversation (SYSREQ-006).

## Exceptions & Edge Cases

- Mapping unknown (merge request opened before installation, or state lost): the event is
  skipped and logged; the service never invents a root (SYSREQ-007).
- Root post deleted by a human: the reply is parked and the thread closed — it stays deleted
  (SYSREQ-026).
- Duplicate re-delivery of the same `webhook-id`: suppressed, no second reply (SYSREQ-014).
