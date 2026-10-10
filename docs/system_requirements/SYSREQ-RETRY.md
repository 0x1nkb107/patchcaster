---
id: "SYSREQ-016"
type: system_requirement
name: "Bounded retry with backoff"
description: "Failed deliveries retry with exponential backoff and rate-limit awareness."
specification: "The service SHALL retry failed Mattermost deliveries with exponential backoff, deferring a rate-limited message until the instant named by its RateLimit-Reset header."
derives_from:
  - "SCEN-005"
---

# System Requirement: Bounded retry with backoff

## Requirement Specification

> The service SHALL retry failed Mattermost deliveries with exponential backoff, deferring a rate-limited message until the instant named by its RateLimit-Reset header.

## Rationale

Mattermost restarts and rolling upgrades make brief failures normal; without retry they would
cost messages. Rate-limit responses are a signal to slow down — ignoring them would prolong the
outage for every sender.

## Acceptance Criteria

- Failed attempts retry with exponential backoff starting at 1 s and doubling to a 10-minute cap;
  after 8 failed attempts the message is parked with a visible counter (constants set by the
  owner, 2026-10-10).
- For an attempt that received `429`, the next attempt is scheduled no earlier than the instant
  in `RateLimit-Reset`, overriding the backoff when later.
- Retry state (attempt count, next-attempt time) persists across restarts.

## Verification Plan

- **Method**: Test.
- **Procedure**: Fake clock with scripted failure/`429` responses from the Mattermost mock;
  assert the exact attempt schedule including `RateLimit-Reset` deferral.

## Notes

The retry schedule is implemented with `cenkalti/backoff` as an interval generator behind the
durable queue (ADR-008); the library choice changes no observable behaviour of this item.
