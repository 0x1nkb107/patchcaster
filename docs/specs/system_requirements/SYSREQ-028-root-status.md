---
id: "SYSREQ-028"
type: system_requirement
name: "Root status line and refresh"
description: "The root message carries a conditional status line and is updated whenever derived state changes."
specification: "The root message SHALL carry a status line rendered from the merge request's derived state, with pipeline, reviewer, comment, and edit segments present only when their signal exists, and the posted root message SHALL be updated within the delivery of every accepted event that changes that state."
derives_from:
  - "SCEN-011"
depends_on:
  - "SYSREQ-004"
  - "SYSREQ-027"
---

# System Requirement: Root status line and refresh

## Requirement Specification

> The root message SHALL carry a status line rendered from the merge request's derived state —
> with pipeline, reviewer, comment, and edit segments present only when their signal exists — and
> the posted root message SHALL be updated within the delivery of every accepted event that
> changes that state.

## Rationale

The root is the triage surface every channel member scans; a second hand-edited line that lies
about the merge request is worse than none. Segments exist only when real so the line never
carries `0 comments`-style noise, and refreshing inside the delivery keeps Mattermost traffic
proportional to actual state change (one root update per accepted event, not per poll).

## Acceptance Criteria

- The status line joins present segments with ` • ` in the order pipeline, reviewers, comments,
  edits; with no present segment the line is absent entirely.
- Pipeline segment: rendered only after the first linked pipeline delivery; emoji and label come
  from a fixed table for known GitLab pipeline statuses (`🟡` running, `🟢` passed, `🔴` failed,
  neutral otherwise); an unrecognised status degrades to a neutral emoji with the raw status text
  and never fails the render.
- Reviewer segment `👀 n/m reviewers` renders only when `m > 0`, with `n` and `m` from the
  reviewer tally (SYSREQ-027).
- Comment segment `💬 n comments` renders only when `n > 0`; edit segment `📝 n edits` renders
  only when `n > 0`.
- Root updates go through the Mattermost posts update operation (SYSREQ-004); failures follow
  the retry ladder (SYSREQ-016), and a root post Mattermost reports missing parks the delivery
  and closes the mapping (SYSREQ-026).
- A retried delivery SHALL NOT post a notice reply that a previous attempt of the same delivery
  already posted, while repeating the (idempotent) root update is safe and allowed.
- After a merge delivery, the final root refresh reflects the merged state before the mapping
  closes (SYSREQ-026 closing semantics apply only afterwards).

## Verification Plan

- **Method**: Test.
- **Procedure**: Golden-text renderer checks per segment and segment combination, including the
  all-absent line and an unknown pipeline status; worker tests over httptest Mattermost doubles
  asserting exactly one notice plus one root PUT per notifying delivery, no root PUT for
  no-change deliveries, and phase preservation across a simulated mid-delivery retry.

## Notes

Root updates carry an "edited" marker in the Mattermost UI; the owner accepted this as the
visible cost of the live status board (2026-10-10).
