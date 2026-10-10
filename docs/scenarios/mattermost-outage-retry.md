---
id: "SCEN-005"
type: scenario
name: "Delivery survives a transient Mattermost outage"
description: "GitLab keeps getting fast acknowledgements while messages queue and drain once Mattermost recovers."
refines:
  - "UC-001"
---

# Scenario: Delivery survives a transient Mattermost outage

## Overview

Mattermost is briefly unavailable while merge request traffic continues; GitLab keeps getting
prompt acknowledgements and nothing is lost.

## Initial State

- The service is healthy; the Mattermost API returns errors or is unreachable.

## Trigger

One or more verified merge request deliveries arrive during the outage.

## Step-by-Step Flow

1. Each delivery verifies, routes, and is persisted as an outbound message before the response.
2. The service answers GitLab 2xx within the acknowledgement budget.
3. Delivery workers fail, back off exponentially, and re-attempt; a 429 response defers the next
   attempt until its `RateLimit-Reset`.
4. When Mattermost recovers, the backlog drains oldest-first.

## Expected Outcome

- Every accepted delivery eventually produces exactly one post.
- Queue depth and attempt counts rise and fall observably in metrics.

## Exceptions & Edge Cases

- A process crash during the outage loses nothing: pending messages are on disk (SYSREQ-017).
- A message that exhausts its retry budget is parked with a metric, not silently discarded.
- A routing-table rollout during the outage redirects nothing: every pending row carries the
  destination resolved at ingress (SYSREQ-024).
