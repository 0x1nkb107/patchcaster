---
id: "SYSREQ-006"
type: system_requirement
name: "Durable merge request to thread mapping"
description: "The merge request to root post mapping is stored durably."
specification: "The service SHALL persistently store the mapping from merge request identity to root post identifier so that thread continuity survives process restarts."
derives_from:
  - "SCEN-002"
---

# System Requirement: Durable merge request to thread mapping

## Requirement Specification

> The service SHALL persistently store the mapping from merge request identity to root post identifier so that thread continuity survives process restarts.

## Rationale

Without durable mapping, every restart splits threads into fresh root posts and the product
quality collapses precisely during rollouts. The mapping is also the join key that makes
`merge`/`close` land in the right conversation months later.

## Acceptance Criteria

- After a root post is accepted by Mattermost, a mapping from merge request identity (project
  identifier + merge request identifier) to root post identifier is written durably.
- Stopping the process and starting it again against the same database still routes later events
  into the original thread.

## Verification Plan

- **Method**: Test.
- **Procedure**: Integration test that runs the service twice sequentially against one
  disposable PostgreSQL database (connection URL from the test environment), posting root in
  run 1 and reply in run 2.

## Notes

Storage technology is decided in ADR-001; this item is deliberately technology-neutral.
