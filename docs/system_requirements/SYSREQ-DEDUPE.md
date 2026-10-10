---
id: "SYSREQ-014"
type: system_requirement
name: "Duplicate delivery suppression"
description: "GitLab re-deliveries of the same webhook identifier are not posted twice."
specification: "The service SHALL acknowledge a delivery whose webhook identifier it already accepted within a retention period of at least 24 hours without posting again."
derives_from:
  - "SCEN-004"
---

# System Requirement: Duplicate delivery suppression

## Requirement Specification

> The service SHALL acknowledge a delivery whose webhook identifier it already accepted within a retention period of at least 24 hours without posting again.

## Rationale

GitLab retries failed deliveries with growing backoff (from ~10 min up to a 24 h ceiling) and
keeps `webhook-id` — equal to `Idempotency-Key` — stable across every retry of the same message
([docs](https://docs.gitlab.com/user/project/integrations/webhooks/)). A stable identifier makes
"already accepted" a durable fact keyed on the delivery itself, so retries become benign:
the second arrival answers `2xx`, which also heals a temporarily disabled webhook.

Retention of 24 h is an owner decision (2026-10-10) sized against the retry backoff ceiling;
paired with the 4 h event-age cap (SYSREQ-013) it closes the replay window completely — see the
arithmetic there.

## Acceptance Criteria

- A verified delivery whose `webhook-id` is present in the seen store answers `2xx` with zero
  posts and increments the suppressed counter.
- Suppression survives a process restart (the seen set lives in the durable store, not process
  memory).
- Seen rows are retained for at least 24 h and pruned only after retention; the pruner is
  bounded work, not a full-table scan per request.
- A delivery cannot bypass dedupe: the signature is computed over `webhook-id`, so a missing or
  altered identifier fails verification first (SYSREQ-012).
- A delivery is recorded as seen only in the same store transaction that durably accepts its
  message or its no-forward decision: a crash can never leave a seen identifier with no enqueued
  message, so suppression can never mask a lost event (pairs with the `5xx` answer of
  SYSREQ-017).

## Verification Plan

- **Method**: Test.
- **Procedure**: Replay the same signed delivery before and after a simulated restart; unit-test
  the pruner against an injected clock; assert suppression counters and zero outbound calls; an
  injected enqueue failure asserts a `5xx` answer and no seen record, so the following GitLab
  retry is processed rather than suppressed.

## Notes

Dedupe runs after signature verification so an attacker cannot poison the seen set with forged
identifiers.
