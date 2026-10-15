---
id: "ADR-034"
type: architecture_decision_record
name: "Explicit teams scope operators"
description: "Administrators create teams, assign them sources, sinks and operators, and operators route only within their teams."
status: proposed
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-006"
---

# ADR-034: Explicit teams scope operators

## Context

The owner defined three roles on 2026-10-15: Administrators are the IT department, Operators are team leads, and Users are individual developers with the fewest privileges. Operators need a boundary. Without one, any team lead could route any repository into any room. The owner chose explicit teams over teams derived from directory or forge groups.

## Decision

- Administrators create teams, assign sources and sinks to them, and name their operators.
- Operators create, change and remove routes, filters and templates between their teams' sources and sinks.
- A source or sink may be assigned to several teams, for example a shared release channel.
- Users need no team. They manage their own identity links and tokens.
- Every check happens in one authorizer, which the web interface and the API share (SYSREQ-039).

## Consequences

- Good: an administrator decides each team's reach explicitly, and nothing changes when a forge or directory group changes.
- Bad: team membership must be kept up to date by hand. Syncing teams from directory groups can come later, without changing the model.
