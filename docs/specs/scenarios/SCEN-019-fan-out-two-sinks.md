---
id: "SCEN-019"
type: scenario
name: "One merge request fans out into two sinks"
description: "A source is routed into a Mattermost channel and a Matrix room. Each sink gets its own thread, and an outage of one delays nothing in the other."
refines:
  - "UC-005"
---

# Scenario: One merge request fans out into two sinks

## Overview

Fan-out multiplies threads, not failure. Each sink keeps its own order and its own retries.

## Initial State

- Routes connect one source to a Mattermost channel and to a Matrix room.

## Trigger

A merge request opens and gathers comments. Meanwhile the Matrix homeserver is unreachable for twenty minutes.

## Step-by-Step Flow

1. Each accepted event becomes one queued message per routed sink.
2. The Mattermost messages go out at once and in order.
3. The Matrix messages wait and retry with backoff. Once the homeserver answers, they go out in their original order.

## Expected Outcome

- Two threads, one per sink, each with its own root board.
- The Mattermost thread was never delayed by the Matrix outage.

## Exceptions & Edge Cases

- A route removed while its messages wait in the queue does not cancel them. They keep the destination resolved when the event was accepted.
