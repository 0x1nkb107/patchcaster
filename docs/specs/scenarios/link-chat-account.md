---
id: "SCEN-023"
type: scenario
name: "A user links a chat account from the chat"
description: "A developer types the link command in the chat platform, logs in to the one-time link the bot returns, and confirms with the code the web page shows."
status: proposed
refines:
  - "UC-006"
---

# Scenario: A user links a chat account from the chat

## Overview

A chat account links only when the same person proves both sides: the chat account through the command and the code, the person through the login.

## Initial State

- Alice can log in through the identity provider. Her Matrix account is not linked yet.

## Trigger

In a direct chat with the bot, Alice sends `!patchcaster link`, or the trigger word an administrator chose for the connection. On Mattermost she would type `/patchcaster link`, the command the bot registered itself.

## Step-by-Step Flow

1. The bot answers in the direct chat with a one-time link that expires after a few minutes. On Mattermost the answer is visible to Alice only.
2. Alice opens the link and logs in. The page names the Matrix account about to be linked and shows a short code.
3. Alice sends `!patchcaster link` followed by the code in the same direct chat.
4. PatchCaster links the account and the bot confirms in the chat.

## Expected Outcome

- Alice's Matrix account is linked to her, and her web profile lists it.
- The audit log records the link.

## Exceptions & Edge Cases

- Mallory sends Alice a link she requested from her own chat account. If Alice opens it, the page names Mallory's account, and nothing links unless Mallory sends the code from her account. Mallory never sees the code, because it appears only on Alice's screen.
- An expired link or code ends the attempt. Alice starts again with the command.
- A command sent in a shared Matrix room is answered in a direct chat, never in the room.
