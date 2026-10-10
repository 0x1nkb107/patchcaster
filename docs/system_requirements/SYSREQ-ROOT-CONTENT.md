---
id: "SYSREQ-003"
type: system_requirement
name: "Root message content"
description: "The root message carries the fields reviewers need to triage from Mattermost."
specification: "The root message SHALL state the merge request title, the author username, the project path, the merge request number, the web URL, and the source and target branches, and SHALL carry the derived status line defined by SYSREQ-028."
derives_from:
  - "SCEN-001"
depends_on:
  - "SYSREQ-002"
---

# System Requirement: Root message content

## Requirement Specification

> The root message SHALL state the merge request title, the author username, the project path, the merge request number, the web URL, and the source and target branches, and SHALL carry the derived status line defined by SYSREQ-028.

## Rationale

The root message is the team's triage surface: a reader decides from it whether to open the merge
request. The merge request description is deliberately excluded — it is long untrusted Markdown
and the main source of channel spam (owner-approved, 2026-10-10; SYSREQ-025 keeps descriptions
unreachable from message templates). Branch names make the direction of the change readable at a
glance (owner example, 2026-10-10). The channel mention header (`@here :bell:`) lives in the root
template itself — the template, not a runtime flag, decides whether a new thread pings the
channel (owner directive, 2026-10-10).

## Acceptance Criteria

- The root message text contains the merge request title, author username, project path, merge
  request number, web URL, and both branch names, each rendered from structured payload fields.
- The root template begins with the channel-mention header defined by the owner's example layout;
  no other template emits a mention.
- The final line is the status line defined by SYSREQ-028, absent when no signal exists.
- Titles containing Markdown or link syntax do not break or spoof the surrounding message layout.
- No message body is constructed from the merge request `description` field.

## Verification Plan

- **Method**: Test.
- **Procedure**: Golden-text checks over the renderer with fixtures including adversarial titles;
  assert presence and escaping of each required field.

## Notes

Field order, emoji, and styling are design decisions (SYSARCH-003); only these five facts are normative.
