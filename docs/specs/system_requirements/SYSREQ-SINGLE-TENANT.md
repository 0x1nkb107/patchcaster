---
id: "SYSREQ-062"
type: system_requirement
name: "One organization per instance"
description: "An instance and its database serve exactly one organization."
specification: "The service SHALL serve exactly one organization per instance and database, and SHALL hold no data of any other organization."
status: proposed
derives_from:
  - "SCEN-029"
---

# System Requirement: One organization per instance

## Requirement Specification

> The service SHALL serve exactly one organization per instance and database, and SHALL hold no data of any other organization.

## Rationale

The owner decided on single tenancy per instance (2026-10-15). With each organization's data in a database of its own, authorization stays about roles and teams, and no query can reach another organization's data (ADR-029).

## Acceptance Criteria

- The data model has no tenant identifier. Every row belongs to the instance's organization.
- Deleting an instance's database removes everything the organization stored.

## Verification Plan

- **Method**: Inspection.
- **Procedure**: Review of the schema and the deployment documentation.
