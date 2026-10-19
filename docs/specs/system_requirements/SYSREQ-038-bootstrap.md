---
id: "SYSREQ-038"
type: system_requirement
name: "Bootstrap and recovery"
description: "A one-time setup link creates the first administrator. A recovery command restores administrator access."
specification: "The service SHALL offer a command that, while no administrator exists, issues a single-use setup link that expires, and a recovery command that grants the administrator role to a named OpenID Connect identity, and SHALL record both in the audit log."
derives_from:
  - "SCEN-013"
---

# System Requirement: Bootstrap and recovery

## Requirement Specification

> The service SHALL offer a command that, while no administrator exists, issues a single-use setup link that expires, and a recovery command that grants the administrator role to a named OpenID Connect identity, and SHALL record both in the audit log.

## Rationale

A fresh instance needs a way in without a default password, and a locked-out instance needs a way back. Both commands need the database credentials, and anyone who has those already controls the instance. So they add no new attack surface (ADR-033).

## Acceptance Criteria

- The setup link works once and expires after one hour.
- No setup link can be issued once an administrator exists.
- Setup completes only after a successful login through the configured provider. The identity that logged in becomes the first administrator.
- The recovery command names the identity by issuer and subject.

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests drive both commands against a test database, covering reuse, expiry and the refusal while an administrator exists.
