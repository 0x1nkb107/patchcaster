---
id: "SYSREQ-061"
type: system_requirement
name: "One owner per Matrix device"
description: "Exactly one process at a time uses a Matrix device's encryption state."
specification: "The service SHALL let exactly one process at a time use the encryption state of a Matrix bot device."
status: proposed
derives_from:
  - "SCEN-028"
depends_on:
  - "SYSREQ-060"
---

# System Requirement: One owner per Matrix device

## Requirement Specification

> The service SHALL let exactly one process at a time use the encryption state of a Matrix bot device.

## Rationale

Encryption sessions advance with every message. Two processes using the same device's state would fork it, and members could no longer decrypt. A rollout briefly runs two pods (SYSREQ-023), so ownership must be explicit.

## Acceptance Criteria

- A second process waits for ownership of a device. It takes over only after the first released it or died.
- Losing ownership stops sending from that process before the state could diverge.

## Verification Plan

- **Method**: Test.
- **Procedure**: A test starts two processes on one connection and asserts that only one sends at any time, and that the second takes over after the first stops.
