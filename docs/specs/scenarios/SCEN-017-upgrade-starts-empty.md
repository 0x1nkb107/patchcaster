---
id: "SCEN-017"
type: scenario
name: "An upgrade from v0.1 starts from an empty database"
description: "v0.2 is an incompatible upgrade. A v0.1 deployment moves to it with a new, empty database and sets up its connections again. The v0.1 threads are abandoned."
status: proposed
refines:
  - "UC-004"
---

# Scenario: An upgrade from v0.1 starts from an empty database

## Overview

The owner decided on 2026-10-16 that v0.2 is an incompatible upgrade. Carrying v0.1's threads over would need identities v0.1 never stored, so the upgrade starts from scratch instead.

## Initial State

- A v0.1 deployment routes GitLab projects into Mattermost channels. Its database holds live threads.

## Trigger

The person who runs the deployment upgrades to v0.2.

## Step-by-Step Flow

1. Following the release notes, they give v0.2 a new, empty database, or drop the old one.
2. They bootstrap the instance (SCEN-013) and set up the connections, teams, sources, sinks and routes again.
3. They point the GitLab hooks at the new connection's endpoint, or let PatchCaster register them.

## Expected Outcome

- New merge request activity opens new threads. Threads from v0.1 receive no further replies.

## Exceptions & Edge Cases

- Started against the old database, v0.2 refuses to start and names the reason and the remedy (SYSREQ-043).
