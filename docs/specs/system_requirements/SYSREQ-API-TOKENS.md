---
id: "SYSREQ-040"
type: system_requirement
name: "API tokens"
description: "API clients authenticate with personal tokens that act with their owner's permissions."
specification: "The service SHALL authenticate API clients by personal access tokens that act with no more than their owner's role and team scope, expire, can be revoked, and are stored only as hashes."
status: proposed
derives_from:
  - "SCEN-022"
---

# System Requirement: API tokens

## Requirement Specification

> The service SHALL authenticate API clients by personal access tokens that act with no more than their owner's role and team scope, expire, can be revoked, and are stored only as hashes.

## Rationale

Automation must not need a person's browser session, and a leaked token must be limited and short-lived.

## Acceptance Criteria

- A token is shown once, when it is created.
- A token never exceeds its owner's current permissions, and losing a role limits the token at once.
- A revoked or expired token answers `401` on its next use.
- Audit entries name the token as well as its owner (SYSREQ-041).

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests cover creation, use, expiry, revocation and a role change of the owner.
