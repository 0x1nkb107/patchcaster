---
id: "SYSARCH-002"
type: system_architecture
name: "Conversation state store"
description: "Embedded SQLite holding thread mappings, seen webhook identifiers, and pending messages."
satisfies:
  - "SYSREQ-006"
  - "SYSREQ-007"
  - "SYSREQ-014"
  - "SYSREQ-017"
---

# System Architecture: Conversation state store

Normative: merge request thread mappings, seen webhook identifiers, and pending outbound messages
live in an embedded SQLite database on a persistent volume.

## Technical Strategy

SQLite via a pure-Go driver so the binary stays CGO-free and statically buildable
(SYSARCH-005). WAL mode; one writer connection, readers share. Schema changes go through goose
migrations executed at startup; all SQL goes through sqlc-generated, type-checked Go — no
string-built queries (ADR-001).

## Static View (Structure)

Three tables: `mr_thread` (project id + merge request id → root post id, channel id),
`seen_webhook` (webhook id, first-seen time; pruned past retention), `outbox` (payload, state
`pending|parked|sent`, attempt count, next attempt time). One Go package owns the SQL; the rest
of the system sees only the generated repository interfaces.

## Dynamic View (Behavior)

Enqueue is a single write inside the ingest request path (bounded, local disk) — the durability
point behind SYSREQ-017. Startup claims nothing; workers claim due `pending` rows at their
convenience. A periodic prune trims `seen_webhook`. Readiness pings writability (SYSREQ-018).
Volume loss is an operational event, not a code path: mappings and queue rebuild from fresh
roots via SYSREQ-007.
