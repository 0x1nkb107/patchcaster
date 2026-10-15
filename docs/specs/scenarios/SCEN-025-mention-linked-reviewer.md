---
id: "SCEN-025"
type: scenario
name: "A linked reviewer is mentioned in the thread"
description: "A notice names the requested reviewers. A reviewer with linked accounts is mentioned on the sink's platform, one without appears by name."
status: proposed
refines:
  - "UC-006"
---

# Scenario: A linked reviewer is mentioned in the thread

## Overview

Mentions turn a thread into a call to action for exactly the people asked to act, and for no one else.

## Initial State

- A route's review-request template mentions the requested reviewers.
- Bob linked his Forgejo and Matrix accounts. Carol linked neither.

## Trigger

Bob and Carol are requested as reviewers of a pull request routed into a Matrix room.

## Step-by-Step Flow

1. The notice's template asks for a mention of each reviewer.
2. For the Matrix sink, Bob becomes a mention of his Matrix account, which notifies him. Carol appears as her plain name.

## Expected Outcome

- Bob is notified in the thread, Carol is not, and nobody else is.

## Exceptions & Edge Cases

- If Bob opted out of mentions, he appears as his plain name too.
- Board refreshes edit the root and never notify anyone again.
- In a Mattermost sink, Bob is mentioned only if he linked a Mattermost account.
