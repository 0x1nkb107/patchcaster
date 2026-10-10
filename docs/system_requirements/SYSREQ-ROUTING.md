---
id: "SYSREQ-009"
type: system_requirement
name: "Routing table resolution"
description: "The destination channel comes from the routing configuration."
specification: "The service SHALL resolve the destination Mattermost channel for every forwarded event from a routing configuration keyed by GitLab project path."
derives_from:
  - "SCEN-006"
  - "SCEN-007"
---

# System Requirement: Routing table resolution

## Requirement Specification

> The service SHALL resolve the destination Mattermost channel for every forwarded event from a routing configuration keyed by GitLab project path.

## Rationale

Project-path keys are stable, human-reviewable, and diffable in GitOps, which numeric project
identifiers are not *(assumed)*. Path-keyed routing also makes one system hook behave exactly like
per-project webhooks.

## Acceptance Criteria

- The destination channel of every forwarded event equals the table entry for the event's
  `project.full_path`; matching is exact and case-sensitive.
- The table is a declarative document (YAML) mounted or provided at startup, and an invalid table
  prevents serving (SYSREQ-022).

## Verification Plan

- **Method**: Test.
- **Procedure**: Table-driven router tests; config-parsing tests for valid and invalid tables.

## Notes

Project renames re-point routing — an operational risk accepted for reviewability.
