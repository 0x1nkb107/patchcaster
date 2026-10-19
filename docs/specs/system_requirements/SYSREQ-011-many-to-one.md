---
id: "SYSREQ-011"
type: system_requirement
name: "Many projects to one channel"
description: "The routing model allows several projects to share a channel."
specification: "Routes SHALL permit several repositories to share the same sink."
derives_from:
  - "SCEN-006"
---

# System Requirement: Many projects to one channel

## Requirement Specification

> Routes SHALL permit several repositories to share the same sink.

## Rationale

Owner requirement (2026-10-10): a team channel aggregates several repositories. The data model
must not assume project-to-channel bijectivity anywhere.

## Acceptance Criteria

- Several routes may lead into the same sink, from one source or from several.
- Events from each of those repositories produce independent threads in the shared sink
  (SCEN-006), and root messages name their project (SYSREQ-003).

## Verification Plan

- **Method**: Test.
- **Procedure**: Config-load test plus a two-project event flow asserting separate threads.

## Notes

Generalized on 2026-10-18 by SYSREQ-046: routes stored in the database replace the routing table.
