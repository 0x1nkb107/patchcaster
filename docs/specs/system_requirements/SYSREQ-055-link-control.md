---
id: "SYSREQ-055"
type: system_requirement
name: "People control their links"
description: "Everyone can list, export and remove their own identity links and tokens."
specification: "The service SHALL let every person list, export and remove their own identity links and tokens, SHALL let Instance Admins remove them for a person who left, and SHALL stop using a removed link at once."
status: proposed
derives_from:
  - "SCEN-026"
---

# System Requirement: People control their links

## Requirement Specification

> The service SHALL let every person list, export and remove their own identity links and tokens, SHALL let Instance Admins remove them for a person who left, and SHALL stop using a removed link at once.

## Rationale

Identity links are personal data. Their owner must be able to see and remove them, and offboarding must not depend on the person who left.

## Acceptance Criteria

- The export is a machine-readable document of the person's links and tokens, without token values.
- A removed link is not used for any message delivered after the removal.
- Removals by an Instance Admin are audited (SYSREQ-041).

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests export and remove links as their owner and as an Instance Admin, and assert that a message delivered afterwards carries no mention.
