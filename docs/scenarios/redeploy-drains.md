---
id: "SCEN-010"
type: scenario
name: "Redeploys drain in flight"
description: "A rolling update finishes in-flight deliveries before the process exits."
refines:
  - "UC-003"
---

# Scenario: Redeploys drain in flight

## Overview

A rolling update swaps the running instance without dropping the deliveries it had already
accepted.

## Initial State

- The instance is serving; at least one delivery is mid-handoff and messages may be queued.

## Trigger

The orchestrator sends SIGTERM during a rollout.

## Step-by-Step Flow

1. The public listener stops accepting new deliveries immediately.
2. In-flight handler work finishes; delivery workers finish or re-park their current attempt with its backoff state.
3. The process exits cleanly, or force-stops at the 30 s budget.

## Expected Outcome

- No accepted delivery is lost: whatever did not post is pending on disk and drains after the new instance starts.
- GitLab sees connection errors only *after* SIGTERM, which is within the rollout plan, not message loss.

## Exceptions & Edge Cases

- A message still undeliverable at force-stop simply remains in the durable queue.
