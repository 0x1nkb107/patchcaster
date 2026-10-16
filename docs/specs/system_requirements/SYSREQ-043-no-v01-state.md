---
id: "SYSREQ-043"
type: system_requirement
name: "No upgrade of v0.1 state"
description: "v0.2 refuses to start on a database that still holds v0.1 state, and it imports nothing from v0.1."
specification: "The service SHALL refuse to start on a database that holds the state of a v0.1 release, naming the reason and the remedy, and SHALL import no configuration, thread or queued message of a v0.1 release."
status: proposed
derives_from:
  - "SCEN-017"
---

# System Requirement: No upgrade of v0.1 state

## Requirement Specification

> The service SHALL refuse to start on a database that holds the state of a v0.1 release, naming the reason and the remedy, and SHALL import no configuration, thread or queued message of a v0.1 release.

## Rationale

The owner decided on 2026-10-16 that v0.2 is an incompatible upgrade, and that v0.1 threads may be abandoned. v0.1 identifies a merge request by GitLab's global id and stores no target repository id, so its threads cannot map onto the new identity of ADR-036. A clear refusal is better than a service that half-understands an old schema.

## Acceptance Criteria

- The service recognizes v0.1's schema by its migration history. It then exits with a non-zero status before serving, and its message says that v0.2 needs a new, empty database.
- The refusal writes nothing to the database.
- The release notes of v0.2 state the incompatibility and the upgrade steps of SCEN-017.

## Verification Plan

- **Method**: Test.
- **Procedure**: A store test builds a v0.1 database from frozen copies of v0.1's migrations and asserts that migration refuses it without writing anything. An end-to-end test starts the binary against a v0.1 database and asserts the non-zero exit, the message and the untouched v0.1 thread.
