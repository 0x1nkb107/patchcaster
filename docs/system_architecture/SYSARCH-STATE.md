---
id: "SYSARCH-002"
type: system_architecture
name: "Conversation state store"
description: "PostgreSQL 18 holding thread mappings, seen webhook identifiers, and pending messages, behind a backend-neutral repository interface."
satisfies:
  - "SYSREQ-006"
  - "SYSREQ-007"
  - "SYSREQ-014"
  - "SYSREQ-017"
  - "SYSREQ-024"
  - "SYSREQ-027"
  - "SYSREQ-030"
---

# System Architecture: Conversation state store

Normative: merge request thread mappings, seen webhook identifiers, and pending outbound
messages live in PostgreSQL 18, reached through a backend-neutral repository interface
(ADR-010).

## Technical Strategy

PostgreSQL 18, accessed with `pgx` (`pgxpool` owns pooling and health; `database/sql` adapter
for sqlc's `engine: postgresql`). All SQL is sqlc-generated and type-checked; schema changes are
goose migrations applied at startup. One internal package implements the store; **no engine type
— sqlc row struct, `pgx` type, or driver error — crosses its boundary**: callers receive domain
structs and repository interfaces (owner directive, 2026-10-10), which is what keeps the
"backend-neutral" promise checkable rather than aspirational.

## Static View (Structure)

Four relations: `mr_thread` (project id + merge request id → root post id, channel id, thread
state open/closed per SYSREQ-026), `mr_state` (one row per merge request: pipeline status, human
comment count, edit count, reviewer tally, source branch and open flag for push attribution, last
attributed push count — SYSREQ-027/030), `seen_webhook` (webhook id, first-seen time; pruned past
retention per SYSREQ-014), `outbox` (payload, resolved destination channel per SYSREQ-024, state
`pending|parked|sent`, attempt count, next attempt time, claim lease, plus per-delivery phase
flags marking which Mattermost operations of a delivery already succeeded — SYSREQ-028). Four
repository interfaces — thread, state, dedupe, outbox — consumed by ingest (enqueue, seen-check,
state apply), delivery (claim, render-read, settle) and by readiness (ping).

## Dynamic View (Behavior)

Enqueue, the seen-mark, and the derived-state application of the same delivery commit together in
one PostgreSQL transaction in the ingest request path (SYSREQ-014, SYSREQ-017, SYSREQ-027) — that
commit is the point where the `2xx` becomes honest, and a failed commit means none of them
happened, leaving GitLab's retry to be processed normally. Ingest is the only writer of derived
state, which is what makes counts exactly-once: delivery workers only read state to render. Workers claim due rows with a lease (`claimed_at` + expiry, so a
crashed worker's rows come back), claiming in an order that serializes each merge request to one
worker at a time, which is what preserves SYSREQ-002's exactly-one-root under the worker pool.
A periodic task prunes `seen_webhook` past retention. Readiness is a pool ping against a writable
store (SYSREQ-018): database unavailability fails readiness, never liveness (SCEN-008). v1
deploys a single replica; the lease design is the groundwork, not the promise, of any future
multi-replica increment.