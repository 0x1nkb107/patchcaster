---
id: "ADR-029"
type: architecture_decision_record
name: "One instance and one database per organization"
description: "Each PatchCaster instance serves exactly one organization from a database of its own."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-006"
---

# ADR-029: One instance and one database per organization

## Context

PatchCaster will connect several forges and chat platforms per instance (UC-007). The owner decided on 2026-10-15 that one instance serves one organization, and that no database holds the data of more than one.

## Decision

- An instance and its database serve exactly one organization. The data model carries no tenant identifier.
- Another organization runs another instance with a database of its own. That database may live on a PostgreSQL server the organization already runs.
- Every instance has its own encryption key (ADR-035), its own public base URL and its own interface URL.

## Consequences

- Good: no query can leak another organization's data, because none is there. Authorization stays about roles and teams only.
- Good: removing an organization means deleting an instance and its database.
- Bad: each organization needs a deployment and a database of its own, even when small.
