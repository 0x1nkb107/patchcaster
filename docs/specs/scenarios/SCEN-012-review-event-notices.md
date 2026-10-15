---
id: "SCEN-012"
type: scenario
name: "Scenario: Notable activity notifies the thread"
refines:
  - "UC-001"
---

# Scenario: Notable activity notifies the thread

## Overview

Beyond the quiet root refresh, the team is actively pulled back to the thread when something
notable happens: someone commented, the pipeline passed or failed, a review was submitted or
changes were requested, an approval landed, new commits were pushed, or the merge request merged.
Everything noisier-but-uninteresting (pipeline merely running, comment edits, reviewer
re-requests, approval resets) stays root-only.

## Initial State

- A merge request thread is announced and mapped (SCEN-001, SYSREQ-006).

## Trigger

An accepted delivery of a notifying kind: new human comment, pipeline success, pipeline failure,
submitted review, requested changes, individual approval, attributed push, or merge.

## Step-by-Step Flow

1. The delivery is verified and deduplicated (SCEN-004).
2. Its merge request is resolved through the stored mapping; unmapped merge requests skip
   silently (SYSREQ-007).
3. Exactly one notice reply is posted into the thread.
4. The root status line is refreshed for the same delivery (SCEN-011 mechanics).

## Expected Outcome

- Each notifying event produces exactly one thread reply; retried deliveries do not duplicate the
  notice.
- Notices read: `💬 <user> commented`, `🟢 Pipeline passed`, `🔴 Pipeline failed`,
  `👀 <user> reviewed the MR`, `🚨 <user> asks for changes!`,
  `✅ Review was approved by <user>!`, `📝 <user> pushed N new commits`,
  `🎉 <user> merged !<iid>`; the commit count is omitted when GitLab gave the bot no count.
- Non-notifying deliveries (pipeline running/pending/manual/canceled/skipped, comment edits,
  unapprovals and system approval resets, reviewer re-requests, label-only edits) post no notice.
