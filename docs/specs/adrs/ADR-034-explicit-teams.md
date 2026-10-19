---
id: "ADR-034"
type: architecture_decision_record
name: "Explicit teams scope Routing Managers"
description: "Instance Admins create teams, assign them sources, sinks and Routing Managers, and Routing Managers route only within their teams."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-006"
---

# ADR-034: Explicit teams scope Routing Managers

## Context

The owner defined three roles on 2026-10-15: Instance Admins are the IT department, Routing Managers are team leads, and Regular Users are individual developers with the fewest privileges. Routing Managers need a boundary. Without one, any team lead could route any repository into any room. The owner chose explicit teams over teams derived from directory or forge groups.

## Decision

- Instance Admins create teams, assign sources and sinks to them, and name their Routing Managers.
- Routing Managers create, change and remove routes, filters and templates between their teams' sources and sinks.
- A source or sink may be assigned to several teams, for example a shared release channel.
- Regular Users need no team. They manage their own identity links and tokens.
- Every check happens in one authorizer, which the web interface and the API share (SYSREQ-039).

## Consequences

- Good: an Instance Admin decides each team's reach explicitly, and nothing changes when a forge or directory group changes.
- Bad: team membership must be kept up to date by hand. Syncing teams from directory groups can come later, without changing the model.

## Notes

Amended on 2026-10-19 after an owner decision: the roles formerly named administrator, operator and user are named Instance Admin, Routing Manager and Regular User.
