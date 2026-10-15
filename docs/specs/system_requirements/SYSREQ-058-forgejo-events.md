---
id: "SYSREQ-058"
type: system_requirement
name: "Forgejo activity mapping"
description: "Forgejo pull request activity maps onto the merge request events that drive threads and boards."
specification: "The service SHALL map Forgejo pull request, comment, review, review request, synchronization, push and Actions run deliveries onto the merge request events that drive threads and boards, identifying each pull request by connection, repository and number."
status: proposed
derives_from:
  - "SCEN-027"
---

# System Requirement: Forgejo activity mapping

## Requirement Specification

> The service SHALL map Forgejo pull request, comment, review, review request, synchronization, push and Actions run deliveries onto the merge request events that drive threads and boards, identifying each pull request by connection, repository and number.

## Rationale

The thread and board behaviour of UC-001 must not depend on the forge. Forgejo's webhook event types, as defined in `modules/webhook/type.go` of its source (checked 2026-10-15), cover the same activity under other names.

## Acceptance Criteria

- `pull_request` opens, edits, closes, merges and reopens the merge request.
- `pull_request_comment` adds a comment.
- `pull_request_review_approved`, `pull_request_review_rejected` and `pull_request_review_comment` record the reviewer's verdict.
- `pull_request_review_request` requests reviewers.
- `pull_request_sync` and `push` record new commits.
- `action_run_success` and `action_run_failure`, or the `workflow_run_*` types, report the pipeline result.
- The kind comes from the `X-Forgejo-Event-Type` header, which Forgejo does not sign. A body whose shape does not fit that kind is refused.
- Required approvals come from the repository's branch protection through the forge API when the connection has API access. Otherwise the board shows only the approvals given.

## Verification Plan

- **Method**: Test.
- **Procedure**: Fixtures captured from a Forgejo instance map to the expected events. An end-to-end test drives a fake Forgejo through SCEN-027.
