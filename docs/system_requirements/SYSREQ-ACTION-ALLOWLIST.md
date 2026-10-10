---
id: "SYSREQ-008"
type: system_requirement
name: "Forwarded action allowlist"
description: "Only the agreed merge request actions are forwarded."
specification: "The service SHALL process only merge request, merge-request comment, pipeline, and push deliveries and SHALL post only merge request announcements, close and merge replies, and the notices of SYSREQ-029, acknowledging every other delivery without posting or state change."
derives_from:
  - "SCEN-003"
---

# System Requirement: Forwarded action allowlist

## Requirement Specification

> The service SHALL process only merge request, merge-request comment, pipeline, and push deliveries and SHALL post only merge request announcements, close and merge replies, and the notices of SYSREQ-029, acknowledging every other delivery without posting or state change.

## Rationale

The noise contract. The posted merge request actions were chosen by the owner on 2026-10-10;
`reopen`, `unapproval`, and `unapproved` are deliberately excluded from posting because
approval-reset storms on pushes would spam channels — they still refresh derived state so the
status board stays honest. Comment, pipeline, and push deliveries are processed for state and for
the narrow notice set of SYSREQ-029 (owner directive, 2026-10-10). Unknown future GitLab kinds and
actions default to silence, not noise.

## Acceptance Criteria

- Processed kinds are `merge_request`, `note` on merge requests, `pipeline`, and `push`; every
  other kind answers `2xx` with no state change and no post.
- Posting merge request actions remain `open`, `close`, `merge`, `approval`, `approved`, and
  `update` (announcements, close/merge replies, and the review-transition notices routed through
  SYSREQ-029); `reopen`, `unapproval`, `unapproved`, or unrecognized actions answer `2xx` with
  zero posts while still refreshing state.
- State-applying deliveries: all recognized merge request actions (including the silent ones),
  human comment creations, linked pipeline deliveries, and attributed pushes (SYSREQ-027,
  SYSREQ-030).
- The allowlist remains a single in-code table for v1, deny-by-default, shaped as data so a
  later move to configuration is additive (owner-approved, 2026-10-10).

## Verification Plan

- **Method**: Test.
- **Procedure**: Parameterized handler tests over every action value documented for GitLab 19
  merge request events.

## Notes

`update` is forwarded regardless of `changes` content in v1; content-based noise filtering is a
possible follow-up, not a gap against this item. Migration to configuration must preserve
deny-by-default: a malformed or empty table forwards nothing, exactly as an empty allowlist
behaves today.
