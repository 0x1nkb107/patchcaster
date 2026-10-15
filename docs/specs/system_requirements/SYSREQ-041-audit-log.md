---
id: "SYSREQ-041"
type: system_requirement
name: "Configuration audit log"
description: "Every configuration change is recorded with who made it, when and how, and administrators can read the record."
specification: "The service SHALL record every configuration change with the acting person and token, the time, the item and the change itself, with secret values redacted, in the same transaction as the change, and SHALL let administrators read the record but not alter it."
status: proposed
derives_from:
  - "SCEN-016"
---

# System Requirement: Configuration audit log

## Requirement Specification

> The service SHALL record every configuration change with the acting person and token, the time, the item and the change itself, with secret values redacted, in the same transaction as the change, and SHALL let administrators read the record but not alter it.

## Rationale

With three roles and an API, "who changed this?" has to have an answer. Writing the entry in the change's own transaction means no change can escape the log.

## Acceptance Criteria

- A change and its audit entry commit or fail together.
- A changed secret appears only as "secret changed", never with a value.
- The web interface and the API offer no way to edit or delete entries.
- Bootstrap, recovery, logins of administrators, and identity link changes are recorded too.

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests assert one entry per change for every operation, including a failed change that must leave none.
