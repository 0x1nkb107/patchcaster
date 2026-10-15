---
id: "SCEN-011"
type: scenario
name: "Scenario: Root message tracks merge request state"
refines:
  - "UC-001"
---

# Scenario: Root message tracks merge request state

## Overview

The announced root message is a living triage surface. As the merge request progresses — pipelines
run, reviewers submit, comments arrive, commits are pushed — the bot edits the root message so a
reader always sees current state at a glance. Signals that do not exist are absent: no pipeline
segment for projects without CI, no comment segment before the first human comment.

## Initial State

- A merge request thread was announced (SCEN-001) and its mapping is stored (SYSREQ-006).
- The project may or may not have CI configured; reviewers may or may not be assigned.

## Trigger

Any state-bearing delivery for that merge request: a linked pipeline status change, a submitted
review, a new human comment, a push to the source branch, or a description edit.

## Step-by-Step Flow

1. The delivery is verified and accepted (SCEN-004 rules), and its derived state is applied.
2. The bot re-renders the root message's status line from current derived state.
3. The root post is updated through the Mattermost API.
4. Pipeline segments appear only from the first pipeline delivery linked to this merge request;
   reviewer, comment, and edit segments appear once their counts are non-zero.

## Expected Outcome

- After every accepted state-bearing delivery, the root message reflects the latest state.
- Deliveries that change no observable state leave the root message content identical.
- The pipeline status board never shows for merge requests whose pipelines GitLab never reported
  to the webhook; unlinked branch pipelines do not create a segment.
- Reviewer progress reads `n/m reviewers` where `m` is the repository policy minimum when
  derivable from the payload, otherwise the number of assigned reviewers.
