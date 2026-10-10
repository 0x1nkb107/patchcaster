---
id: "SYSARCH-003"
type: system_architecture
name: "Mattermost delivery worker"
description: "Background workers consuming the durable queue and posting via REST v4."
satisfies:
  - "SYSREQ-002"
  - "SYSREQ-003"
  - "SYSREQ-004"
  - "SYSREQ-005"
  - "SYSREQ-016"
  - "SYSREQ-025"
  - "SYSREQ-026"
  - "SYSREQ-028"
  - "SYSREQ-029"
---

# System Architecture: Mattermost delivery worker

Normative: delivery runs in background workers that consume the durable queue and post through
the Mattermost REST API v4.

## Technical Strategy

A small pool of workers polls the outbox for due `pending` rows (claim → attempt → settle). The
Mattermost client is a narrow wrapper over `POST /api/v4/posts` and `PUT /api/v4/posts/{id}`
with an explicit per-request
timeout, typed errors for retryable vs. terminal outcomes (ADR-005), and no other API surface —
keeping the trust boundary one file wide. Message text is rendered by templates in code, not by
interpolating payload text (SYSREQ-003).

## Static View (Structure)

Components: `Renderer` (named `text/template` templates over the escaped view model —
SYSREQ-025, ADR-009), `MattermostClient`
(API v4 posts: create and update only), `Worker` (claim loop, backoff policy from SYSREQ-016),
`Store` repository (SYSARCH-002). Configuration supplies site URL and bot token (SYSREQ-022); the
token reaches the client only, never the renderer or logging path.

## Dynamic View (Behavior)

Worker claims a due row, resolves `mr_thread` (root vs. reply decision → SYSREQ-002,
SYSREQ-005): no mapping skips the row with a log (SYSREQ-007), a closed thread skips it likewise,
and Mattermost's missing-root answer parks the row and closes the thread (SYSREQ-026). A delivery
then runs as two idempotent phases over current `mr_state`: the notice reply `POST` — skipped
when the row's phase flag marks it posted — followed by the root status `PUT` (SYSREQ-028);
failures at either phase retry under the same ladder without re-running the completed one. Only
the merge delivery's final root update may settle, after which the mapping closes.
Otherwise it posts, then settles the row: `sent`, rescheduled with backoff or
`RateLimit-Reset` deferral (SYSREQ-016), or `parked` after the attempt budget. Drain (SYSREQ-023)
stops claiming and lets in-flight attempts finish.