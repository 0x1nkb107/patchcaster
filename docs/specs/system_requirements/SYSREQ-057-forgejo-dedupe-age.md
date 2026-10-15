---
id: "SYSREQ-057"
type: system_requirement
name: "Forgejo duplicates and event age"
description: "Forgejo deliveries are de-duplicated by a digest of their signed body and capped by the event time inside that body."
specification: "The service SHALL suppress a Forgejo delivery whose verified body it already accepted, judged by a digest of that body, and SHALL apply the event age cap of SYSREQ-013 to the event time inside the body for every payload kind that carries one."
status: proposed
derives_from:
  - "SCEN-027"
depends_on:
  - "SYSREQ-013"
  - "SYSREQ-014"
---

# System Requirement: Forgejo duplicates and event age

## Requirement Specification

> The service SHALL suppress a Forgejo delivery whose verified body it already accepted, judged by a digest of that body, and SHALL apply the event age cap of SYSREQ-013 to the event time inside the body for every payload kind that carries one.

## Rationale

Forgejo's delivery id travels in an unsigned header, so a replayed body could carry a fresh one. Only the signed body is trustworthy, so duplicates are judged by its digest. Forgejo signs no timestamp, so the age cap can rest only on times inside the body, as it already does for GitLab (ADR-041).

## Acceptance Criteria

- A verified body seen within the duplicate retention of SYSREQ-014 is acknowledged with `2xx` and has no effect, whatever its delivery id header says.
- Payload kinds with an event time in the body are capped by it. The Forgejo adapter's documentation lists each kind and field.
- Payload kinds without one, such as pushes, are accepted on duplicate suppression alone. That weaker guarantee is stated in ADR-041.

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests replay a signed body with a new delivery id, and drive the age cap per payload kind with an injected clock.
