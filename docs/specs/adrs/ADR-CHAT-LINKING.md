---
id: "ADR-039"
type: architecture_decision_record
name: "Chat accounts link through a command and a code sent back"
description: "A user starts linking with a chat command, logs in through the link the bot returns privately, and confirms by sending back the code the login shows."
status: proposed
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-006"
  - "SYSARCH-007"
---

# ADR-039: Chat accounts link through a command and a code sent back

## Context

The owner decided that users start identity linking through the chat platform's command capabilities, for example `/bot link` (2026-10-15). If the bot's answer were a link that completes the linking on its own, an attacker could request a link from their own chat account and send it to a victim. When the victim opens it while logged in, the attacker's chat account is linked to the victim, and the attacker receives the victim's mentions, review requests included.

## Decision

- The command starts the flow. The bot answers privately with a single-use link valid for ten minutes.
- The link requires login. The page names the chat account about to be linked and shows a single-use code, valid for ten minutes.
- The user sends the code back from the same chat account. Only then does the link exist. An attacker never sees the victim's code, so a forwarded link links nothing.
- On Mattermost the command is the custom slash command `/patchcaster`, and the answers are visible to the requester only. The bot registers the command itself, so no one has to set it up by hand and get it wrong (owner decision 2026-10-15).
  - It creates the command through `POST /api/v4/commands` in every team it belongs to, pointing at the connection's command endpoint, and seals the token Mattermost returns (ADR-035). Every command request must carry that token.
  - It checks its registrations when the connection starts, when a sink is added and periodically. It re-creates a command someone deleted, and removes its commands together with the connection.
  - This needs the `manage_slash_commands` permission in those teams, and a Mattermost server that allows custom slash commands and requests to PatchCaster's address.
- On Matrix, clients treat unknown slash commands as typing errors, so the command is a message such as `!patchcaster link` in a direct chat with the bot, read from the sync stream. Administrators may override the trigger word per connection, and `!patchcaster` is the default (owner decision 2026-10-15). A command in a shared room is answered in a direct chat.
- Failed codes are rate-limited per chat account.

## Consequences

- Good: both sides of every link are proven by the same person.
- Good: the flow needs no direct messages beyond the ones linking itself sends.
- Bad: linking takes one more step than a plain link would.
