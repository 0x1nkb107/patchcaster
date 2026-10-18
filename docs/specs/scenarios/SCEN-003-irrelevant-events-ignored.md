---
id: "SCEN-003"
type: scenario
name: "Irrelevant events are ignored"
description: "Events outside the forwarded scope are acknowledged without producing channel noise."
refines:
  - "UC-001"
---

# Scenario: Irrelevant events are ignored

## Overview

Deliveries that carry nothing to forward — other event kinds, non-allowlisted actions, or
projects that no route reaches — are acknowledged and produce no channel noise.

## Initial State

- The service is running with its routes stored and its allowlist built in.

## Trigger

Any of: a `push`/`pipeline`/comment delivery; a merge request delivery with action `reopen`,
`unapproval`, or `unapproved`; a merge request delivery from an unconfigured project path.

## Step-by-Step Flow

1. The signature verifies as usual (authenticity is checked before content decisions).
2. The kind/action filter or the routing lookup decides nothing must be forwarded.
3. The service responds 2xx and increments the ignored-delivery counters.

## Expected Outcome

- No Mattermost post; no queue entry.
- GitLab treats the delivery as successful and does not retry.

## Exceptions & Edge Cases

- The exclusions of `reopen`, `unapproval`, and `unapproved` are an owner scope decision
  (2026-10-10), not a technical limitation.
