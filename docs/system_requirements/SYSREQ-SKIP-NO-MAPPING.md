---
id: "SYSREQ-007"
type: system_requirement
name: "Skip events without thread mapping"
description: "Events for merge requests with no stored thread mapping are skipped and logged, never re-rooted."
specification: "The service SHALL skip, log, and count every forwarded event whose merge request has no stored thread mapping."
derives_from:
  - "SCEN-002"
depends_on:
  - "SYSREQ-006"
---

# System Requirement: Skip events without thread mapping

## Requirement Specification

> The service SHALL skip, log, and count every forwarded event whose merge request has no stored thread mapping.

## Rationale

Owner directive (2026-10-10), replacing the earlier substitute-root default: a merge request the
bot never announced must not gain a mid-life root post — readers would land in a conversation
with no opening, and an MR that predates the webhook installation deserves silence plus an
explainable counter, not a synthetic announcement of something already underway.

Two signals are kept apart. No mapping found is a *policy skip*: the event is complete, correct,
and simply not ours to post. A lookup that *errors* (store unavailable) is a transient failure:
the message stays pending and retries under SYSREQ-016 — "retry safely" applies to that case,
never to the policy case.

## Acceptance Criteria

- A verified, routed event whose mapping lookup finds nothing produces zero posts, one structured
  log record naming the `webhook-id`, project, merge request reference, and skip reason, and a
  distinct skip counter increment (SYSREQ-019).
- A failing mapping lookup (store unavailable) leaves the message pending; after recovery it is
  retried under SYSREQ-016, never dropped.
- No code path creates a substitute root post for an unmapped merge request.

## Verification Plan

- **Method**: Test.
- **Procedure**: Empty store plus a `merge` event asserts skip, counter, and zero Mattermost
  calls; an injected store fault asserts the outbox row survives the worker and retries after
  recovery.

## Notes

The skip counter is a deployment runbook feature: merge requests opened before the webhook was
installed produce expected skips, not errors. This policy also governs state loss (SYSARCH-002):
threads are never rebuilt.
