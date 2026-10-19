---
id: "SCEN-026"
type: scenario
name: "A user removes their links"
description: "A developer reviews, exports and removes their own identity links. Mentions stop at once."
status: proposed
refines:
  - "UC-006"
---

# Scenario: A user removes their links

## Overview

Identity links are personal data. Their owner controls them.

## Initial State

- Alice has linked a Matrix account and a Forgejo account.

## Trigger

Alice opens her profile.

## Step-by-Step Flow

1. The profile lists her links and API tokens.
2. Alice exports them, then removes the Matrix link.

## Expected Outcome

- Later notices name Alice without mentioning her.
- The audit log records the removal.

## Exceptions & Edge Cases

- When someone leaves the organization, an Instance Admin can remove their links and tokens. That too is audited.
