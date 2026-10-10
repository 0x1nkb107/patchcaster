---
id: "SYSREQ-001"
type: system_requirement
name: "Merge request event ingestion"
description: "Accept and parse GitLab merge request webhook payloads from all hook scopes."
specification: "The service SHALL accept GitLab merge request, comment, pipeline, and push webhook deliveries from project, group, and system hooks, parsing timestamp fields as ISO 8601 values as emitted by GitLab 19 and later."
derives_from:
  - "SCEN-001"
  - "SCEN-007"
---

# System Requirement: Merge request event ingestion

## Requirement Specification

> The service SHALL accept GitLab merge request webhook deliveries from project, group, and system hooks, parsing timestamp fields as ISO 8601 values as emitted by GitLab 19 and later.

## Rationale

One parse-and-accept boundary gives every downstream component a single normalized event model.
GitLab 19.0 changed webhook timestamps to ISO 8601 with millisecond precision, and system-hook
deliveries carry an extra top-level discriminator; absorbing those variants here keeps routing and
delivery simple.

## Acceptance Criteria

- Recorded GitLab 19 fixtures for project, group, and system-hook merge request, comment,
  pipeline, and push events all parse into the internal event model, retaining each kind's
  merge request linkage (merge request id; note and pipeline payloads name it, push deliveries
  carry project and branch for SYSREQ-030 attribution).
- Timestamps of the form `2026-04-23T12:30:45.123Z` parse and the event time is retained.
- Malformed JSON or missing required fields (`object_kind`, `object_attributes.action`, project
  path) answer `400` and are never queued.

## Verification Plan

- **Method**: Test.
- **Procedure**: Table-driven testify tests (arrange-act-assert) over fixtures recorded from a
  GitLab 19 instance, one per hook scope, plus malformed-input cases against the HTTP handler.

## Notes

Payloads in the pre-19.0 timestamp format are non-conforming input for v1 and take the same
`400` path; GitLab 19+ is an explicit solution constraint.
