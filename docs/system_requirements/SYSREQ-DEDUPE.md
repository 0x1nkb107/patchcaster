---
id: "SYSREQ-014"
type: system_requirement
name: "Duplicate delivery suppression"
description: "GitLab re-deliveries of the same webhook identifier are not posted twice."
specification: "The service SHALL acknowledge a delivery whose webhook identifier was already accepted within the freshness window without posting again."
derives_from:
  - "SCEN-004"
---

# System Requirement: Duplicate delivery suppression

## Requirement Specification

> The service SHALL acknowledge a delivery whose webhook identifier was already accepted within the freshness window without posting again.

## Rationale

GitLab re-delivers when it times out or sees an error; a duplicate reply is user-visible noise on
every thread. `webhook-id` is unique per delivery attempt sequence, making it a safe natural key.

## Acceptance Criteria

- A delivery whose `webhook-id` was already accepted answers `2xx` with zero posts and increments
  the suppressed counter.
- Seen identifiers are stored durably: suppression survives a process restart inside the window.
- Seen-identifier retention covers at least 24 h, then rows may be pruned *(assumed — GitLab
  retries can arrive long after the freshness window)*.

## Verification Plan

- **Method**: Test.
- **Procedure**: Store unit tests plus handler tests replaying a stored delivery before and after
  a simulated restart.

## Notes

Suppression is checked after signature verification so an attacker cannot poison the dedupe set.
