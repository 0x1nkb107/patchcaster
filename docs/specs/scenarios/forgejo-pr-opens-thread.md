---
id: "SCEN-027"
type: scenario
name: "A Forgejo pull request opens a thread"
description: "Forgejo pull request activity verifies, de-duplicates and maps onto the same thread and board behaviour as GitLab merge requests."
status: proposed
refines:
  - "UC-007"
---

# Scenario: A Forgejo pull request opens a thread

## Overview

Forgejo signs its webhooks differently from GitLab. The thread behaviour must not differ.

## Initial State

- A Forgejo connection with API access, a source for the `payments` organization, and a route into a sink.

## Trigger

A pull request opens, is approved, its Forgejo Actions run succeeds, and it is merged.

## Step-by-Step Flow

1. Each delivery verifies against the connection's secret (SYSREQ-056).
2. Each is checked against earlier deliveries by a digest of its body, and against the age cap by the event time inside the body (SYSREQ-057).
3. Each maps onto the merge request events PatchCaster already handles (SYSREQ-058).
4. The thread opens, the approval and the pipeline result reply, and the merge closes the board.

## Expected Outcome

- The thread reads like a GitLab merge request's thread.

## Exceptions & Edge Cases

- A captured delivery replayed with a new `X-Forgejo-Delivery` header is a duplicate, because the body digest repeats.
- A delivery with a missing or wrong signature answers `401`.
- Push deliveries carry no event time of their own. They rely on duplicate suppression alone.
- The board's required approvals come from the forge API. Without API access, the board shows only the approvals given.
