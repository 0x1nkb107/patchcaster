---
id: "UC-004"
type: use_case
name: "Instance administration"
description: "The organization's IT department sets up and governs one PatchCaster instance: login, connections to forges and chat platforms, teams with their sources, sinks and operators."
refines:
  - "SOL-001"
---

# Use Case: Instance administration

## Actor(s)

- Administrator: the organization's IT department.
- The person who deploys the instance, for the first start only.

## Pre-conditions

- The instance runs with its database, its encryption key, its public base URL and its interface URL.
- The organization has an OpenID Connect identity provider.
- The administrators hold credentials for the forges and chat platforms the instance will use.

## Main Success Outcome

An administrator logs in through the organization's identity provider and connects the forges and chat platforms the organization uses. They create teams, give each team its sources and sinks, and name its operators. Afterwards they can trace every configuration change. None of this needs a redeploy.

## Key Functional Scope

- The first start issues a one-time setup link. The first administrator configures the identity provider and claims the role by logging in through it (SYSREQ-037, SYSREQ-038).
- Connections to forges and chat platforms, with credentials stored encrypted and never shown again (SYSREQ-042).
- Sources and sinks, assigned to the teams that may use them (SYSREQ-044, SYSREQ-045).
- Teams and their operators (SYSREQ-039).
- An audit log of every configuration change (SYSREQ-041).
- An upgrade from v0.1 starts from an empty database, and the service refuses a database that still holds v0.1 state (SYSREQ-043).

Out of scope:

- Passwords or user management of PatchCaster's own. The identity provider decides who someone is.
- Teams derived from directory or forge groups. Teams are explicit (ADR-034).
- Several organizations in one instance (ADR-029).

## Post-conditions

- The database holds the complete configuration. Only startup and operating settings live outside it (SYSREQ-034).
