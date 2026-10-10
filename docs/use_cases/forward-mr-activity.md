---
id: "UC-001"
type: use_case
name: "Merge request activity in Mattermost threads"
description: "Team members see GitLab merge request activity as threaded conversations in the Mattermost channels they already work in."
refines:
  - "SOL-001"
---

# Use Case: Merge request activity in Mattermost threads

## Actor(s)

- GitLab instance (webhook deliveries, signed per Standard Webhooks).
- Mattermost channel subscribers (readers of the threads).
- PatchCaster service (verifier, filter, router, poster).

## Pre-conditions

- The GitLab project is registered in the routing configuration with a destination channel.
- The webhook is registered with a signing token and the merge request trigger enabled.
- The Mattermost bot account is a member of the destination channel.

## Main Success Outcome

The channel receives a root message when a merge request is opened, and threaded replies for
allowlisted subsequent activity, each naming the actor, action, and merge request.

## Key Functional Scope

- Parse merge request payloads from project, group, and system hooks (GitLab 19 timestamp formats).
- Verify delivery authenticity and event age; suppress duplicates.
- Forward exactly the actions `open`, `close`, `merge`, `approval`, `approved`, `update`.
- One thread per merge request, keyed by a durable merge-request-to-post mapping.

Out of scope for v1 (deliberate, owner decision 2026-10-10):

- Non-merge-request events (`push`, `pipeline`, comments, emoji, wiki, work items).
- The `reopen`, `unapproval`, `unapproved` actions.
- Editing or deleting already-posted messages (posts are append-only).
- Posting merge request descriptions or diffs (root and replies carry structured fields only, SYSREQ-003).
- Noise filtering inside `update` (all updates forwarded regardless of `changes` content).

## Post-conditions

- An open thread exists per announced merge request in the mapped channel.
- The service has persisted the mapping and acknowledged every accepted delivery to GitLab.
