---
id: "UC-005"
type: use_case
name: "Team routing and message design"
description: "Routing Managers decide which of their team's sources announce into which of their team's sinks, which events reach the threads, and how the messages read."
refines:
  - "SOL-001"
---

# Use Case: Team routing and message design

Accepted in part on 2026-10-18, with increment 1 (SOL-001). Still planned, and not normative until their increment is accepted: templates per route, written in Markdown and previewed (SYSREQ-049 to SYSREQ-051), with increment 2.

## Actor(s)

- Routing Manager: a team lead.
- Automation that acts with a Routing Manager's API token.

## Pre-conditions

- An Instance Admin created the team, named the Routing Manager, and assigned the team its sources and sinks (UC-004).

## Main Success Outcome

Activity from the team's repositories lands in the chat destinations the Routing Manager chose, with the replies the Routing Manager wants and in the wording of the Routing Manager's templates. One source may announce into several sinks, and several sources into one sink.

## Key Functional Scope

- Routes between the sources and sinks of the Routing Manager's team (SYSREQ-046), with one thread per merge request and sink (SYSREQ-048).
- Route filters that silence thread replies while the root board still follows every event (SYSREQ-047).
- Templates per route, written in Markdown, validated and previewed before they apply (SYSREQ-049, SYSREQ-050, SYSREQ-051).
- Every operation available through the API as well as the web interface (SYSREQ-035, SYSREQ-040).

Out of scope:

- Filters by branch or label. Filters act on event kinds first.
- Routes between sources and sinks of different teams, except sinks an Instance Admin assigned to several teams.

## Post-conditions

- A routing change applies to events accepted after it. Queued messages keep the destination resolved when they were accepted (SYSREQ-024).
- This use case replaces the file-driven routing of UC-002 (2026-10-18).
