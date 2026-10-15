---
id: "SYSREQ-063"
type: system_requirement
name: "Chat commands"
description: "The bot registers and keeps its Mattermost slash command itself, and Matrix connections accept the trigger word an administrator chose."
specification: "The service SHALL register its slash command in every Mattermost team its bot belongs to and keep that registration in place, SHALL accept a Mattermost command request only with the token Mattermost issued for that registration, and SHALL recognize on Matrix the trigger word an administrator configured for the connection, `!patchcaster` by default."
status: proposed
derives_from:
  - "SCEN-014"
  - "SCEN-023"
---

# System Requirement: Chat commands

## Requirement Specification

> The service SHALL register its slash command in every Mattermost team its bot belongs to and keep that registration in place, SHALL accept a Mattermost command request only with the token Mattermost issued for that registration, and SHALL recognize on Matrix the trigger word an administrator configured for the connection, `!patchcaster` by default.

## Rationale

The owner decided on 2026-10-15 that the bot registers its Mattermost command itself, to avoid human error. Mattermost commands belong to a team, so the bot needs one registration per team. Matrix has no server-side commands, so the trigger word is a message prefix that administrators may change per connection (ADR-039).

## Acceptance Criteria

- Adding a Mattermost connection registers `/patchcaster` in each of the bot's teams. A missing `manage_slash_commands` permission is reported with the team and the reason.
- A registration someone deleted is re-created at the next check, and removing the connection removes its registrations.
- A command request with a missing or wrong token answers `401` and has no effect.
- Only administrators can change a Matrix connection's trigger word, and the change is audited (SYSREQ-041).

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests drive registration, re-creation and removal against the fake Mattermost, which validates the calls against Mattermost's published commands description, as ADR-027 does for posts. A command request with a wrong token must be refused. A Matrix test changes the trigger word and sends both the old and the new one.
