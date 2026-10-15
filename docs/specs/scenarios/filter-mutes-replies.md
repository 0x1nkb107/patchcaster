---
id: "SCEN-020"
type: scenario
name: "A route filter mutes replies but not the board"
description: "A route filters out comments. Comments no longer reply in the thread, but the root board still counts them."
status: proposed
refines:
  - "UC-005"
---

# Scenario: A route filter mutes replies but not the board

## Overview

Filters reduce noise in a thread. They never make the board lie about the merge request.

## Initial State

- A route's filter excludes comments. Approvals and pipeline results pass.

## Trigger

A reviewer comments on the merge request and later approves it.

## Step-by-Step Flow

1. The comment updates the root board but posts no reply.
2. The approval updates the root board and posts a reply.

## Expected Outcome

- The board shows the same state as in an unfiltered route's sink.
- The thread holds only the replies the filter lets through.

## Exceptions & Edge Cases

- The root is always created when a merge request opens, whatever the filter says. It is the thread's anchor.
