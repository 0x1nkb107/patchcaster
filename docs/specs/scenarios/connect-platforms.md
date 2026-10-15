---
id: "SCEN-014"
type: scenario
name: "An administrator connects a forge and a chat platform"
description: "An administrator adds a Forgejo instance and a Matrix homeserver as connections. Their credentials are checked, stored encrypted and never shown again."
status: proposed
refines:
  - "UC-004"
---

# Scenario: An administrator connects a forge and a chat platform

## Overview

Connections carry the credentials PatchCaster acts with. Everything else, sources and sinks included, builds on them.

## Initial State

- An administrator is logged in.
- They hold a Forgejo API token and the access token and recovery key of a Matrix bot account.

## Trigger

The administrator adds a forge connection and a chat connection.

## Step-by-Step Flow

1. The administrator enters the Forgejo URL and token. PatchCaster calls the forge API, shows the account the token acts as, and creates the connection's webhook endpoint and secret.
2. The administrator enters the homeserver URL, the bot's access token and its recovery key. PatchCaster logs in and cross-signs its device with the recovery key.
3. PatchCaster stores the secrets encrypted. The interface offers to replace them, never to show them.

## Expected Outcome

- Both connections are listed with their state: reachable, authenticated and, for Matrix, verified.
- The audit log records both additions with the secrets redacted.

## Exceptions & Edge Cases

- A token the platform rejects is refused, with the platform's answer.
- A wrong recovery key is refused. An unverified device would show warnings in every encrypted room.
- A Mattermost connection works the same way, with the bot account's token. PatchCaster then registers its `/patchcaster` command in every team the bot belongs to. If the bot may not manage slash commands in a team, the connection names that team and the reason.
- For a Matrix connection, the administrator may change the trigger word of the bot's commands. The default is `!patchcaster`.
- A forge connection without an API token is possible. PatchCaster then shows the webhook endpoint and secret once, when it creates them, so an administrator can register the hooks by hand.
