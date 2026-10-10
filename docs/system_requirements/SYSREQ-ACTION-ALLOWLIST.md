---
id: "SYSREQ-008"
type: system_requirement
name: "Forwarded action allowlist"
description: "Only the agreed merge request actions are forwarded."
specification: "The service SHALL forward only the merge request actions open, close, merge, approval, approved, and update, acknowledging every other delivery without posting."
derives_from:
  - "SCEN-003"
---

# System Requirement: Forwarded action allowlist

## Requirement Specification

> The service SHALL forward only the merge request actions open, close, merge, approval, approved, and update, acknowledging every other delivery without posting.

## Rationale

The noise contract. The six forwarded actions were chosen by the owner on 2026-10-10;
`reopen`, `unapproval`, and `unapproved` are deliberately excluded because approval-reset storms
on pushes would spam channels. Unknown future GitLab actions default to silence, not noise.

## Acceptance Criteria

- Verified deliveries with action `open`, `close`, `merge`, `approval`, `approved`, or `update`
  proceed to routing.
- Any other action value — `reopen`, `unapproval`, `unapproved`, or unrecognized — answers `2xx`
  with zero posts.
- The allowlist is a single in-code table for v1, deny-by-default — unrecognised actions never
  post — and shaped as data so a later move to configuration is additive (owner-approved,
  2026-10-10).

## Verification Plan

- **Method**: Test.
- **Procedure**: Parameterized handler tests over every action value documented for GitLab 19
  merge request events.

## Notes

`update` is forwarded regardless of `changes` content in v1; content-based noise filtering is a
possible follow-up, not a gap against this item. Migration to configuration must preserve
deny-by-default: a malformed or empty table forwards nothing, exactly as an empty allowlist
behaves today.
