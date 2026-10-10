---
id: "ADR-001"
type: architecture_decision_record
name: "SQLite with sqlc and goose for state"
description: "Use embedded SQLite for thread continuity and the outbound queue, sqlc for query code, goose for migrations."
status: superseded
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-002"
---

# Architecture Decision: SQLite with sqlc and goose for state

> **Superseded by ADR-010 (2026-10-10).** PostgreSQL replaces SQLite as the state backend;
> sqlc and goose are retained under the new decision.

## Context and problem statement

Thread continuity (SYSREQ-006), duplicate suppression (SYSREQ-014), and the durable outbox
(SYSREQ-017) require state that survives restarts. The service is meant to be one container and
one small volume; the decision is which storage technology carries that state and how its SQL is
managed.

## Key factors

- Single static Go binary, CGO-free build preferred (SYSARCH-005).
- Tiny write volume (webhook rate), local durability, no cross-replica coordination today.
- Owner directive (2026-10-10): if the project has persistence, start with SQLite, generate query
  code with sqlc, version schema with goose.

## Considered options

- Embedded SQLite + sqlc + goose.
- External PostgreSQL.
- In-memory state with recovery via Mattermost post search.

## Decision Outcome

Chosen option: "Embedded SQLite with sqlc-generated queries and goose migrations", because it
keeps the deployment a single binary plus a volume, gives transactional guarantees for the
outbox, and the owner's toolchain directive removes the codegen and migration questions.

### Positive Consequences

- No external service dependency; crash-consistent WAL storage; offline-testable persistence.
- sqlc makes every query compile-checked; goose migrations run at startup unattended.

### Negative Consequences

- Single-writer store ⇒ single replica (volume is ReadWriteOnce); horizontal scale would need a
  store swap later — mitigated by isolating SQL in one package behind repository interfaces.
- Volume loss loses mappings and backlog; mitigated operationally (backup of one small file).
  After loss, unmapped events are skipped and logged (SYSREQ-007) — threads are not rebuilt.

## Pros and Cons of the Options

### Embedded SQLite + sqlc + goose

- Good, because one artifact and one file to back up.
- Good, because transactions cover enqueue-before-ack (SYSREQ-017).
- Bad, because scaling beyond one writer requires a redesign.

### External PostgreSQL

- Good, because it scales horizontally and is familiar operationally.
- Bad, because it adds a second stateful service to every deployment for a workload measured in
  events per second.

### In-memory state

- Good, because it is the simplest code.
- Bad, because restarts split threads and crash-loses queued messages, violating SYSREQ-006/017.

## Links

- Justifies SYSARCH-002.
- Driver choice (pure-Go `modernc.org/sqlite`) recorded in SYSARCH-002 *(assumed, reversible)*.