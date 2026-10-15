---
id: "SYSREQ-043"
type: system_requirement
name: "Import of file configuration"
description: "The file configuration of a v0.1 release moves into the database once, and existing threads continue."
specification: "The service SHALL import the routing file, signing secrets and Mattermost settings of a v0.1 release into an empty configuration database in one transaction, keeping the existing webhook path, secrets and thread mappings."
status: proposed
derives_from:
  - "SCEN-017"
depends_on:
  - "SYSREQ-034"
---

# System Requirement: Import of file configuration

## Requirement Specification

> The service SHALL import the routing file, signing secrets and Mattermost settings of a v0.1 release into an empty configuration database in one transaction, keeping the existing webhook path, secrets and thread mappings.

## Rationale

v0.1 deployments must cross from file configuration to database configuration without re-registering GitLab hooks or losing their threads.

## Acceptance Criteria

- The import creates a GitLab connection without API access, answering on the v0.1 webhook path with the v0.1 signing secrets.
- It creates one source per routing entry, scoped to that entry's project, and the matching Mattermost connection, sinks and routes with the built-in templates.
- Existing thread mappings point at their new sinks, so the next delivery replies in the old thread.
- The import refuses to run when configuration exists, and an invalid input aborts it without writing anything.
- It reports everything it created.

## Verification Plan

- **Method**: Test.
- **Procedure**: An end-to-end test runs v0.1, opens a thread, upgrades with the import, and asserts that the next comment replies in the same thread.
