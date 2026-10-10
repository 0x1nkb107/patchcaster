---
id: "ADR-014"
type: architecture_decision_record
name: "Architecture Decision: Webhook-derived merge request state"
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-002"
  - "SYSARCH-003"
---
# Architecture Decision: Webhook-derived merge request state

## Context and problem statement

The 2026-10-10 feature extension turns the root message into a live status board (pipeline,
reviewers, comments, edits) and adds thread notices for notable activity (SCEN-011, SCEN-012,
SYSREQ-027 through SYSREQ-030). Every field the board shows must come from somewhere: GitLab
webhooks deliver most of it, but counts drift, events race, and webhooks retry. The decision is
how the service derives and applies this state.

## Key factors

- Owner directive (2026-10-10): derive state from webhooks only; the GitLab read API stays out of
  the runtime (no API call was needed for any board field except push commit counts, and the
  owner chose push-hook attribution over API reads).
- At-least-once ingestion: the same event may be delivered repeatedly; a delivery may crash
  between posting and settling. Naive counters at delivery time double-count.
- Event order is not guaranteed: GitLab fires push and merge-request-update webhooks nearly
  simultaneously; either may be accepted first.
- GitLab 19+ payloads carry per-reviewer `state` and reviewer-state transitions inside `update`
  events — review tallies and verdict notices need no API.
- Mattermost post creation has no idempotency key (SYSREQ-017), but post update by id is
  idempotent.

## Considered options

- Apply derived state at ingestion, inside the seen-mark + enqueue transaction; render at
  delivery from stored state.
- Apply derived state at delivery time in the worker.
- Refresh state by calling the GitLab API when rendering (poll-on-event hybrid).

## Decision Outcome

Chosen option: "apply at ingestion, render at delivery". The ingest transaction — seen-mark,
state application, enqueue — commits exactly once per webhook identifier, making every counter
exactly-once by construction; delivery retries then merely re-render idempotent Mattermost
operations. Per-delivery phase flags on the outbox row distinguish "notice already posted" from
"root already refreshed", so a mid-delivery retry never duplicates a notice while the root PUT may
freely repeat and always converges to current state. Race handling is honest rather than clever:
push notices carry a commit count only when the matching push delivery was applied at acceptance
(SYSREQ-030), and pipeline segments appear only for linked pipeline events. The reviewer tally is
recomputed from every merge-request payload (never incremented), so approval resets and revoking
pushes self-heal on the next MR event even though `unapproval` deliveries post no notice.

### Positive Consequences

- Counters cannot double-count, and Mattermost outages never roll back or re-apply state.
- The worker's delivery unit is two idempotent phases — safe to retry under the existing ladder
  (SYSREQ-016) without new machinery.
- Zero GitLab API surface: no token scopes beyond the webhook secret, no outbound read load, no
  API failure modes in the delivery path.

### Negative Consequences

- Comment counts are upper bounds (no note-deletion webhook exists); the requirements state this
  explicitly (SYSREQ-027).
- Push notices occasionally omit the count when the update event is accepted before the push
  delivery lands, and always omit it for fork sources (SYSREQ-030) — accepted over API reads.
- Root messages accumulate an "edited" marker in Mattermost's UI.

## Pros and Cons of the Options

### Apply at ingestion (chosen)

- Good, because exactly-once ingestion semantics transfer to state for free.
- Good, because rendering-at-delivery makes Mattermost failures harmless to data.
- Bad, because the ingest transaction does slightly more work before answering `2xx`.

### Apply at delivery

- Bad, because claims and retries make "once per event" the worker's problem — phase bookkeeping
  for counters, not just for posts.

### GitLab API refresh

- Good, because counts are exact (deletions, forks).
- Bad, because it was explicitly declined (owner): new outbound dependency, auth surface, and
  failure modes for marginal precision.

## Links

- Implements SCEN-011/SCEN-012; justifies SYSARCH-002 (state relations) and SYSARCH-003 (phased
  delivery).
- Depends on the identity rule (SYSREQ-001) and extends SYSREQ-008's deny-by-default posture to
  new event kinds.
