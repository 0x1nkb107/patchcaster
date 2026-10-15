---
id: "SCEN-024"
type: scenario
name: "A user links a forge account"
description: "A developer connects their Forgejo account from the web interface through Forgejo's OAuth authorization."
status: proposed
refines:
  - "UC-006"
---

# Scenario: A user links a forge account

## Overview

Mentions need to know which forge user is which person. The forge itself proves it through OAuth.

## Initial State

- Bob is logged in. An administrator registered an OAuth application for the Forgejo connection.

## Trigger

Bob chooses to connect his Forgejo account.

## Step-by-Step Flow

1. PatchCaster sends Bob to Forgejo's authorization page, asking only to read his identity.
2. Bob approves, and Forgejo returns him with the account he is logged in as.
3. PatchCaster links that account to Bob.

## Expected Outcome

- Bob's forge account is linked, and his profile lists it.

## Exceptions & Edge Cases

- A forge account already linked to another person is refused.
- Declining on Forgejo's page links nothing.
