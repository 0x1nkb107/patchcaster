---
id: "SYSREQ-011"
type: system_requirement
name: "Many projects to one channel"
description: "The routing model allows several projects to share a channel."
specification: "The routing configuration SHALL permit several GitLab projects to share the same Mattermost channel."
derives_from:
  - "SCEN-006"
---

# System Requirement: Many projects to one channel

## Requirement Specification

> The routing configuration SHALL permit several GitLab projects to share the same Mattermost channel.

## Rationale

Owner requirement (2026-10-10): a team channel aggregates several repositories. The data model
must not assume project-to-channel bijectivity anywhere.

## Acceptance Criteria

- A routing table where several entries name the same channel loads without error.
- Events from each of those projects produce independent threads in the shared channel
  (SCEN-006), and root messages name their project (SYSREQ-003).

## Verification Plan

- **Method**: Test.
- **Procedure**: Config-load test plus a two-project event flow asserting separate threads.

## Notes

None.
