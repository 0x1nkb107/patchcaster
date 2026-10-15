---
id: "SYSREQ-053"
type: system_requirement
name: "Forge identity linking"
description: "A forge account links to a person only through the forge's OAuth authorization."
specification: "The service SHALL link a forge account to a person only through that forge's OAuth authorization, requesting no more than the permission to read the account's identity, and SHALL link each forge account to at most one person."
status: proposed
derives_from:
  - "SCEN-024"
---

# System Requirement: Forge identity linking

## Requirement Specification

> The service SHALL link a forge account to a person only through that forge's OAuth authorization, requesting no more than the permission to read the account's identity, and SHALL link each forge account to at most one person.

## Rationale

Forges have no chat commands, but they are OAuth providers. The forge itself then proves which account a person controls.

## Acceptance Criteria

- An administrator registers one OAuth application per forge connection.
- The authorization requests read access to the user's identity only.
- A forge account already linked to someone else is refused.

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests against a fake OAuth provider cover a link, a refusal, a declined authorization and an attempt to link a taken account.
