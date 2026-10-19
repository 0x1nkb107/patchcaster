---
id: "SCEN-014"
type: scenario
name: "An Instance Admin connects a forge and a chat platform"
description: "An Instance Admin adds a Forgejo instance and a Matrix homeserver as connections. Their credentials are checked, stored encrypted and never shown again."
refines:
  - "UC-004"
---

# Scenario: An Instance Admin connects a forge and a chat platform

Accepted in part on 2026-10-18, with increment 1 (SOL-001). Still planned, and not normative until their increment is accepted: the Matrix connection with its recovery key and trigger word, with increment 4, and the registration of the `/patchcaster` command, with increment 5. Until the Forgejo adapter exists, the scenario holds for GitLab.

## Overview

Connections carry the credentials PatchCaster acts with. Everything else, sources and sinks included, builds on them.

## Initial State

- An Instance Admin is logged in.
- They hold a Forgejo API token and the access token and recovery key of a Matrix bot account.

## Trigger

The Instance Admin adds a forge connection and a chat connection.

## Step-by-Step Flow

1. The Instance Admin enters the Forgejo URL and token. PatchCaster calls the forge API, shows the account the token acts as, and shows the connection's webhook endpoint. Hooks that PatchCaster registers through the forge API get signing secrets it generates.
2. The Instance Admin enters the homeserver URL, the bot's access token and its recovery key. PatchCaster logs in and cross-signs its device with the recovery key.
3. PatchCaster stores the secrets encrypted. The interface offers to replace them, never to show them.

## Expected Outcome

- Both connections are listed with their state: reachable, authenticated and, for Matrix, verified.
- The audit log records both additions with the secrets redacted.

## Exceptions & Edge Cases

- A token the platform rejects is refused, with the platform's answer.
- A wrong recovery key is refused. An unverified device would show warnings in every encrypted room.
- A Mattermost connection works the same way, with the bot account's token. PatchCaster then registers its `/patchcaster` command in every team the bot belongs to. If the bot may not manage slash commands in a team, the connection names that team and the reason.
- For a Matrix connection, the Instance Admin may change the trigger word of the bot's commands. The default is `!patchcaster`.
- A forge connection without an API token is possible. The Instance Admin then registers the hooks by hand at the connection's webhook endpoint, and adds to the connection the signing secret the forge generated for each hook. GitLab's hook form only generates signing tokens and shows each once, so the forge, not PatchCaster, creates these secrets (owner decision 2026-10-17).
- A forge connection holds several signing secrets at once: one per hook registered by hand, or an old and a new one during a rotation. PatchCaster lists them by label, never by value.
