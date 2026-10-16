---
id: "SYSREQ-027"
type: system_requirement
name: "Merge request derived state"
description: "Pipeline, comment, edit, and reviewer state derived from webhook deliveries, applied exactly once per accepted delivery."
specification: "The service SHALL maintain per-merge-request derived state - pipeline status, human comment count, edit count, and reviewer tally - applying it exactly once per accepted delivery, atomically with duplicate suppression and queue insertion."
derives_from:
  - "SCEN-011"
  - "SCEN-012"
depends_on:
  - "SYSREQ-014"
  - "SYSREQ-017"
---

# System Requirement: Merge request derived state

## Requirement Specification

> The service SHALL maintain per-merge-request derived state — pipeline status, human comment
> count, edit count, and reviewer tally — applying it exactly once per accepted delivery,
> atomically with duplicate suppression and queue insertion.

## Rationale

Root refreshes (SYSREQ-028) and notices (SYSREQ-029) must read one coherent snapshot, and
webhook retries make "apply at delivery time" double-count. Applying state in the ingestion
transaction makes application exactly-once by construction: the seen-mark guard already admits
each webhook identifier once, and a Mattermost failure then never rolls state back or re-applies
it (ADR-014).

## Acceptance Criteria

- Pipeline state is set to the status of the most recently accepted pipeline delivery linked to
  the merge request (SYSREQ-030); no pipeline state exists before the first linked delivery.
- The comment count increments by exactly one per accepted non-system merge-request comment
  delivery with action `create`; comment edits leave it unchanged. Comment deletion has no
  webhook: the count MAY drift upward and the service SHALL treat it as an upper bound.
- The edit count increments by one per source-branch push (`oldrev` present or attributed push)
  and per description change; title and label edits do not count.
- The reviewer tally is recomputed — never incremented — from the `reviewers` array of each
  accepted merge-request delivery: numerator counts reviewers in state `reviewed` or `approved`;
  denominator is the approval-rule minimum when present in the payload, otherwise the number of
  assigned reviewers.
- State application commits in the same database transaction as the seen-mark and the enqueue
  (SYSREQ-014, SYSREQ-017); redelivery of a known identifier changes no state.
- Delivery retries never re-apply state: at delivery time the job renders from stored state.

## Verification Plan

- **Method**: Test.
- **Procedure**: Store contract tests over state transitions (comment create/update, pushes via
  `oldrev` and attributed push deliveries, description vs title edits, reviewer recomputation
  including approval resets); an ingestion test replaying a duplicated webhook identifier and
  asserting byte-identical state.

## Notes

The exact comment count is an upper bound by design: GitLab emits no note-deletion webhook
(2026-10-10 owner review of the webhook catalogue).
