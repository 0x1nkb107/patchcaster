---
id: "SYSREQ-046"
type: system_requirement
name: "Routes from sources to sinks"
description: "Each accepted event reaches every sink a route connects to its source: one source to many sinks, many sources to one sink."
specification: "The service SHALL forward each accepted event to every sink that a route connects to a source the event matched."
derives_from:
  - "SCEN-018"
  - "SCEN-019"
depends_on:
  - "SYSREQ-044"
  - "SYSREQ-045"
---

# System Requirement: Routes from sources to sinks

## Requirement Specification

> The service SHALL forward each accepted event to every sink that a route connects to a source the event matched.

## Rationale

Routes replace the routing table. They allow fan-out into several sinks as well as the many-to-one routing of SYSREQ-011.

## Acceptance Criteria

- An event that reaches one sink through several routes is delivered there once.
- An event matching a source with no route is acknowledged without effect.
- A route connects only a source and a sink of the same team (SYSREQ-039).

## Verification Plan

- **Method**: Test.
- **Procedure**: Table-driven tests over source, route and sink layouts assert the set of queued messages per event.

## Notes

Since its acceptance on 2026-10-18, this replaces SYSREQ-009 and generalizes SYSREQ-011.
