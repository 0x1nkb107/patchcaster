---
id: "SCEN-006"
type: scenario
name: "Several projects share one channel"
description: "Two configured projects report into the same channel without their threads interfering."
refines:
  - "UC-002"
---

# Scenario: Several projects share one channel

## Overview

Two projects route into one team channel; each merge request keeps its own thread and the
conversations never cross.

## Initial State

- The routing table maps `group/alpha` and `group/beta` to the same channel identifier.

## Trigger

A merge request is opened in `group/alpha`; shortly after, another is opened in `group/beta`;
both receive subsequent updates.

## Step-by-Step Flow

1. Each delivery resolves to the shared channel via its own routing entry.
2. Each `open` creates its own root post; each update replies within its own root.

## Expected Outcome

- Two independent threads in one channel; each message names its project path so readers can tell them apart.
- No event ever resolves to the other project's root post.

## Exceptions & Edge Cases

- Simultaneous opens are independent transactions; ordering between the two threads is irrelevant.
