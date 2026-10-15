---
id: "SYSREQ-047"
type: system_requirement
name: "Route filters act on replies only"
description: "A route's filter decides which events reply in the thread. The root board still follows every event."
specification: "The service SHALL apply a route's event filter only to thread replies, so that a filtered event still updates the root post in that route's sink."
status: proposed
derives_from:
  - "SCEN-020"
depends_on:
  - "SYSREQ-046"
---

# System Requirement: Route filters act on replies only

## Requirement Specification

> The service SHALL apply a route's event filter only to thread replies, so that a filtered event still updates the root post in that route's sink.

## Rationale

The owner decided on 2026-10-15 that filters suppress only replies. The board then shows the true state of the merge request in every sink, however a route is filtered.

## Acceptance Criteria

- Filters choose among the reply kinds PatchCaster posts: comments, review verdicts, pipeline results, pushes, merges and closes.
- The root is created when a merge request opens, whatever the filter says.
- Two sinks of the same source, one filtered and one not, show the same board.

## Verification Plan

- **Method**: Test.
- **Procedure**: An end-to-end test routes one source into a filtered and an unfiltered sink and compares their boards and replies.
