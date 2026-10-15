---
id: "SYSREQ-048"
type: system_requirement
name: "Threads and ordering per merge request and sink"
description: "Each sink has its own thread per merge request, and an outage of one sink delays no other."
specification: "The service SHALL keep one thread per merge request and sink, and SHALL order and retry deliveries per merge request and sink, so that a failing sink delays no delivery to any other sink."
status: proposed
derives_from:
  - "SCEN-019"
depends_on:
  - "SYSREQ-006"
  - "SYSREQ-017"
---

# System Requirement: Threads and ordering per merge request and sink

## Requirement Specification

> The service SHALL keep one thread per merge request and sink, and SHALL order and retry deliveries per merge request and sink, so that a failing sink delays no delivery to any other sink.

## Rationale

v0.1 keys threads and the queue's ordering holdback by merge request alone. With fan-out, a Matrix outage would then stall the same merge request's Mattermost thread.

## Acceptance Criteria

- A merge request is identified by its forge connection, repository and number (ADR-036).
- The holdback that keeps a merge request's messages in order applies per merge request and sink.
- A sink outage of any length leaves the other sinks' delivery times unchanged.

## Verification Plan

- **Method**: Test.
- **Procedure**: An end-to-end test fails one fake sink for a period and asserts the other's deliveries are on time and both stay in order.

## Notes

On acceptance this extends SYSREQ-006 and SYSREQ-017 from the merge request to the pair of merge request and sink.
