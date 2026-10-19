---
id: "SCEN-028"
type: scenario
name: "An encrypted Matrix room follows a merge request"
description: "The bot posts a thread into an end-to-end encrypted room as a verified device, keeps its keys across a restart, and shares keys with members who join later."
status: proposed
refines:
  - "UC-007"
---

# Scenario: An encrypted Matrix room follows a merge request

## Overview

Encrypted rooms are the default for private rooms in common Matrix clients. The bot must be a well-behaved encrypted participant.

## Initial State

- The Matrix connection is verified: the bot's device is cross-signed (SCEN-014).
- The bot was invited into an encrypted room, which is a sink of a route.

## Trigger

A merge request opens. Later a new member joins the room, and PatchCaster restarts.

## Step-by-Step Flow

1. The bot encrypts the root and its replies for the room members' devices.
2. Board refreshes are encrypted edits of the root.
3. The new member's devices receive the keys for messages sent after they joined.
4. After the restart, the bot continues with the same device and the same keys from the database.

## Expected Outcome

- Members read every message, marked as sent by a verified device.
- The restart is invisible to the room.

## Exceptions & Edge Cases

- If the homeserver is unreachable, messages wait and retry like any sink's (SCEN-019).
- A member's device that cannot receive keys cannot read the messages. The failure is logged per device.
- If the bot's encryption state is lost, for example by restoring an old database backup, an Instance Admin re-verifies the connection with the recovery key. The bot then appears as a new device.
