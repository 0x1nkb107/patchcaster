---
id: "SYSREQ-039"
type: system_requirement
name: "Roles and team scope"
description: "Administrators, operators and users may do different things, and operators act only within their teams."
specification: "The service SHALL authorize every configuration operation by role: administrators for connections, teams, sources, sinks and role assignments; operators for routes, filters and templates between the sources and sinks of their own teams; and users for their own identity links and tokens."
derives_from:
  - "SCEN-015"
  - "SCEN-018"
---

# System Requirement: Roles and team scope

## Requirement Specification

> The service SHALL authorize every configuration operation by role: administrators for connections, teams, sources, sinks and role assignments; operators for routes, filters and templates between the sources and sinks of their own teams; and users for their own identity links and tokens.

## Rationale

The owner defined three roles on 2026-10-15. Administrators are the IT department, operators are team leads, and users are individual developers with the fewest privileges. Explicit teams scope the operators (ADR-034).

## Acceptance Criteria

- Authorization is decided in one place, which the web interface and the API share. Neither surface can grant what the other refuses.
- An operator of one team can neither read nor change another team's routes. The API answers `403`.
- A sink or source assigned to several teams can be routed by each of those teams' operators.
- Administrators may do everything, and every person may manage their own links and tokens.

## Verification Plan

- **Method**: Test.
- **Procedure**: A table-driven test runs every operation as each role, inside and outside the team, through both surfaces.
