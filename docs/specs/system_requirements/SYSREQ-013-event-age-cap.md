---
id: "SYSREQ-013"
type: system_requirement
name: "Event age cap"
description: "Verified deliveries whose event is older than 4 hours are discarded."
specification: "The service SHALL acknowledge verified deliveries whose event timestamp is older than 4 hours with HTTP 2xx without posting."
derives_from:
  - "SCEN-004"
---

# System Requirement: Event age cap

## Requirement Specification

> The service SHALL acknowledge verified deliveries whose event timestamp is older than 4 hours with HTTP 2xx without posting.

## Rationale

Staleness is judged on *when the action happened*, not when GitLab chose to deliver it: the event
timestamp rides inside the HMAC-covered body (`object_attributes.timestamp`, ISO 8601 from
GitLab 19), so it is authentic without trusting delivery timing. This lets legitimate retries
arrive — GitLab backs off from ~10 min to a 24 h ceiling on failures ([retry
behavior](https://docs.gitlab.com/user/project/integrations/webhooks/)) — while news older than
4 h never reaches a channel. The 4 h cap is an owner decision (2026-10-10); the delivery-time
`webhook-timestamp` freshness check it replaces had been 300 s.

The answer for an authentic-but-expired delivery is `2xx` + discard, *not* `4xx`: four
consecutive failures temporarily disable a GitLab webhook and forty disable it permanently
([docs](https://docs.gitlab.com/user/project/integrations/webhooks/)). A policy discard is not an
authentication failure; acknowledging ends GitLab's retry cycle harmlessly.

The cap and the dedupe retention (SYSREQ-014, ≥ 24 h) interlock: a captured delivery replayed
within retention is suppressed as a duplicate; replayed after retention, the event is necessarily
≥ 24 h old and the 4 h cap refuses it. Replay therefore becomes impossible once an event is 4 h
old, and the seen-identifier set never needs to outlive that horizon.

## Acceptance Criteria

- A verified delivery whose payload event timestamp is more than 4 h older than service time
  answers `2xx`, produces zero posts and zero queue rows, and increments an expired-delivery
  counter.
- A verified delivery within 4 h proceeds to routing unaffected.
- Age is read from `object_attributes.timestamp` with `object_attributes.updated_at` as fallback;
  unparseable timestamps are treated as expired.
- Signature verification still precedes age evaluation: unverified deliveries answer `401`
  regardless of age (SYSREQ-012).
- The cap is one named constant or configuration value (4 h).

## Verification Plan

- **Method**: Test.
- **Procedure**: Injected clock over signed fixtures at 4 h ± 1 s boundaries, asserting the `2xx`
  answer, the expired counter, and the absence of side effects; a mixed run asserting `401` still
  wins for bad signatures on old events.

## Notes

Future-dated event timestamps within a small tolerance pass; abuse of that tolerance requires the
signing secret, which the signature already excludes.