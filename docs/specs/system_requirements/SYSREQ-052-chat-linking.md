---
id: "SYSREQ-052"
type: system_requirement
name: "Chat identity linking"
description: "A chat account links to a person only after a command from that account, a login, and a code sent back from the same account."
specification: "The service SHALL link a chat account to a person only after that chat account requested a link through the platform's command, the person logged in through a single-use link the bot returned privately, and the same chat account sent back the code that the login showed."
status: proposed
derives_from:
  - "SCEN-023"
---

# System Requirement: Chat identity linking

## Requirement Specification

> The service SHALL link a chat account to a person only after that chat account requested a link through the platform's command, the person logged in through a single-use link the bot returned privately, and the same chat account sent back the code that the login showed.

## Rationale

The owner decided that users start linking from the chat platform (2026-10-15). A link alone would be open to link injection: an attacker forwards their own link, and the victim's account would receive the attacker's chat account. The code sent back from the same chat account closes that gap (ADR-039).

## Acceptance Criteria

- The link is single-use and expires after ten minutes. The code is single-use and expires after ten minutes.
- A code sent from any other chat account links nothing.
- On Mattermost the bot answers with a message only the requester sees. On Matrix it answers in a direct chat, never in a shared room.
- Failed codes are rate-limited per chat account.

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests cover the full flow on each platform's fake, the forwarded-link attack, expiry, reuse and the rate limit.
