---
id: "SCEN-001"
type: scenario
name: "New merge request opens a thread"
description: "A developer opens a merge request in a configured project and the mapped channel receives a new root message."
refines:
  - "UC-001"
---

# Scenario: New merge request opens a thread

## Overview

A developer opens a merge request in a configured project; the mapped Mattermost channel receives
a new root message naming the merge request, and the service remembers the thread.

## Initial State

- The project path has a routing entry pointing at a channel.
- The merge request has never been seen by the service (no stored mapping).

## Trigger

GitLab delivers a merge request webhook with `object_attributes.action` = `open`, signed with a
configured signing token.

## Step-by-Step Flow

1. GitLab POSTs the payload with `webhook-id`, `webhook-timestamp`, `webhook-signature` headers.
2. The service verifies the signature and timestamp, deduplicates the identifier, and parses the payload.
3. The action is allowlisted and the project resolves to a channel, so the outbound message is persisted.
4. The service acknowledges the delivery to GitLab.
5. A delivery worker posts a root message through the Mattermost bot API and stores the
   merge-request-to-post mapping.

## Expected Outcome

- Exactly one root post (no `root_id`) in the mapped channel containing title, author, project
  path, merge request number, and URL.
- The mapping is durable before any subsequent event is handled.

## Exceptions & Edge Cases

- Mattermost unreachable: the message stays queued and retries (SCEN-005); GitLab still got its 2xx.
- Invalid signature or stale timestamp: rejected per SCEN-004; nothing is queued.
