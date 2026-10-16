---
id: "SYSREQ-029"
type: system_requirement
name: "Review-event thread notices"
description: "Comments, pipeline outcomes, review verdicts, pushes, and merges post exactly one thread notice each."
specification: "The service SHALL post exactly one thread notice for each accepted delivery of: new human comment, pipeline success, pipeline failure, submitted review, requested changes, individual approval, attributed source-branch push, and merge; every other accepted delivery SHALL post no notice and refresh only the root status line."
derives_from:
  - "SCEN-012"
depends_on:
  - "SYSREQ-007"
  - "SYSREQ-027"
  - "SYSREQ-028"
---

# System Requirement: Review-event thread notices

## Requirement Specification

> The service SHALL post exactly one thread notice for each accepted delivery of: new human
> comment, pipeline success, pipeline failure, submitted review, requested changes, individual
> approval, attributed source-branch push, and merge; every other accepted delivery SHALL post no
> notice and refresh only the root status line.

## Rationale

Thread replies pull readers back with a ping; the root refresh does not. The notifying set is the
owner's explicit list (2026-10-10) — moments a teammate acts on — while high-frequency or
intermediate states (pipeline `running`, comment edits, reviewer re-requests, approval resets on
push) would drown the very thread they announce. Silence by default keeps the allowlist posture
(SYSREQ-008): new GitLab event kinds stay root-only until judged noteworthy.

## Acceptance Criteria

- Notice texts are the template-rendered forms: `💬 <user> commented`, `🟢 Pipeline passed`,
  `🔴 Pipeline failed`, `👀 <user> reviewed the MR`, `🚨 <user> asks for changes!`,
  `✅ Review was approved by <user>!`, `📝 <user> pushed <count> new commits`, and
  `🎉 <user> merged !<iid>`; `<count>` is omitted when no commit count was attached at
  acceptance (SYSREQ-030).
- A review submission is detected from the reviewer state transition (`reviewed`,
  `requested_changes`, or `approved`) inside an `update` delivery; the individual approval notice
  is the `approval` action, and the full-approval `approved` action refreshes the root only.
- Deliveries that post no notice include: comment edits (`update` action on a note), all pipeline
  statuses other than `success` and `failed`, `unapproval`/`unapproved` including
  system `approvals_reset_on_push`, reviewer re-requests, and title/label-only updates.
- Human-only comments notify: deliveries flagged `system: true` never produce a notice, so
  GitLab's own notes (including "added 1 commit" system notes) stay out of the thread.
- Every notice targets the thread's own root (SYSREQ-005 semantics) and is bound by the
  no-duplicate-on-retry rule (SYSREQ-028) and the unmapped-skip rule (SYSREQ-007).
- The merge notice precedes the final root refresh and the mapping close.

## Verification Plan

- **Method**: Test.
- **Procedure**: Table-driven worker tests over fixtures for each notifying kind and each listed
  silent kind, asserting exactly-one-notice, notice text, and root-update pairing; adversarial
  fixtures (system notes, review transition embedded in unrelated update payloads).

## Notes

Username placeholders come from the delivery's acting user, never from note bodies; bodies of
comments are not posted (description rule SYSREQ-003 extended: untrusted Markdown stays out).
