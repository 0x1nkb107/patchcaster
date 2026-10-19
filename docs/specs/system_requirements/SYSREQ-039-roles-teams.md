---
id: "SYSREQ-039"
type: system_requirement
name: "Roles and team scope"
description: "Instance Admins, Routing Managers and Regular Users may do different things, and Routing Managers act only within their teams."
specification: "The service SHALL authorize every configuration operation by role: Instance Admins for connections, teams, sources, sinks and role assignments; Routing Managers for routes, filters and templates between the sources and sinks of their own teams; and Regular Users for their own identity links and tokens."
derives_from:
  - "SCEN-015"
  - "SCEN-018"
---

# System Requirement: Roles and team scope

## Requirement Specification

> The service SHALL authorize every configuration operation by role: Instance Admins for connections, teams, sources, sinks and role assignments; Routing Managers for routes, filters and templates between the sources and sinks of their own teams; and Regular Users for their own identity links and tokens.

## Rationale

The owner defined three roles on 2026-10-15. Instance Admins are the IT department, Routing Managers are team leads, and Regular Users are individual developers with the fewest privileges. Explicit teams scope the Routing Managers (ADR-034).

## Acceptance Criteria

- Authorization is decided in one place, which the web interface and the API share. Neither surface can grant what the other refuses.
- A Routing Manager of one team can neither read nor change another team's routes. The API answers `403`.
- A sink or source assigned to several teams can be routed by each of those teams' Routing Managers.
- Instance Admins may do everything, and every person may manage their own links and tokens.

## Verification Plan

- **Method**: Test.
- **Procedure**: A table-driven test runs every operation as each role, inside and outside the team, through both surfaces.

## Notes

Amended on 2026-10-19 after an owner decision: the roles formerly named administrator, operator and user are named Instance Admin, Routing Manager and Regular User.
