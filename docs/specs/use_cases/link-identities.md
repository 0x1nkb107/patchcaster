---
id: "UC-006"
type: use_case
name: "Linked identities for mentions"
description: "Developers link their forge and chat accounts, so that threads mention them where a template names them, for example as a requested reviewer."
status: proposed
refines:
  - "SOL-001"
---

# Use Case: Linked identities for mentions

## Actor(s)

- User: an individual developer, with the fewest privileges.

## Pre-conditions

- The user can log in through the organization's identity provider.
- An administrator connected the user's forge and chat platform.

## Main Success Outcome

A user links a chat account from inside the chat platform and a forge account from the web interface. From then on, threads mention the user's chat account wherever a template names them.

## Key Functional Scope

- Chat accounts link through a chat command and a code sent back from the same account (SYSREQ-052).
- Forge accounts link through the forge's OAuth authorization (SYSREQ-053).
- Mentions render per sink platform, and people without a link appear by name (SYSREQ-054).
- Users see, export and remove their own links (SYSREQ-055).

Out of scope:

- Direct messages beyond the ones linking needs. They come later.

## Post-conditions

- Every link was proven by its owner on both sides: the login proves the person, and the platform proves the account.
