---
id: "SCEN-018"
type: scenario
name: "A Routing Manager routes a source into a sink"
description: "A team lead connects the team's Forgejo source to the team's Matrix room. The next pull request opens a thread there."
refines:
  - "UC-005"
---

# Scenario: A Routing Manager routes a source into a sink

## Overview

Routing is the Routing Manager's everyday task. It needs no Instance Admin and no redeploy.

## Initial State

- The team "Payments" owns a Forgejo source and a Matrix sink (SCEN-015).

## Trigger

The team lead, as the team's Routing Manager, creates a route.

## Step-by-Step Flow

1. The Routing Manager picks the source and the sink from the team's lists.
2. The Routing Manager keeps the default filter, which lets every event through, and the built-in templates.
3. PatchCaster saves the route and records it in the audit log.

## Expected Outcome

- The next pull request opened in the `payments` organization opens a thread in the room.

## Exceptions & Edge Cases

- Sources and sinks of other teams are not offered. Naming one through the API answers `403`.
- Deliveries accepted before the route existed are not replayed into it.
