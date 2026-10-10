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

Three relations: `mr_thread` (project id + merge request id → root post id, channel id, thread
state open/closed per SYSREQ-026), `seen_webhook` (webhook id, first-seen time; pruned past
retention per SYSREQ-014), `outbox` (payload, resolved destination channel per SYSREQ-024, state
`pending|parked|sent`, attempt count, next attempt time, claim lease). Three repository
interfaces — thread, dedupe, outbox — consumed by ingest (enqueue, seen-check) and delivery
(claim, settle) and by readiness (ping).

## Dynamic View (Behavior)

Enqueue is a single INSERT in the ingest request path (SYSREQ-017) — its commit is the point
where the `2xx` becomes honest. Workers claim due rows with a lease (`claimed_at` + expiry, so a
crashed worker's rows come back), claiming in an order that serializes each merge request to one
worker at a time, which is what preserves SYSREQ-002's exactly-one-root under the worker pool.
A periodic task prunes `seen_webhook` past retention. Readiness is a pool ping against a writable
store (SYSREQ-018): database unavailability fails readiness, never liveness (SCEN-008). v1
deploys a single replica; the lease design is the groundwork, not the promise, of any future
multi-replica increment.
