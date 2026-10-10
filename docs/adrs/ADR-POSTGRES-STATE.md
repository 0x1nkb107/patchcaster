---
id: "ADR-010"
type: architecture_decision_record
name: "PostgreSQL 18 with pgx for state"
description: "Conversation state, dedupe records, and the durable outbox live in PostgreSQL 18, accessed via pgx, with sqlc queries and goose migrations behind a backend-neutral repository interface."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-002"
supersedes:
  - "ADR-001"
---

# Architecture Decision: PostgreSQL 18 with pgx for state

## Context and problem statement

ADR-001 chose embedded SQLite on a persistent volume. The owner decided (2026-10-10) to
externalize state instead: target PostgreSQL 18, drive it with `pgx`, retain `sqlc` and `goose`,
and require the store package to expose a backend-neutral repository interface. The deployment
consequences of the embedded choice — ReadWriteOnce volume, single-replica coupling, volume
backup as the thread-loss insurance — are what this decision removes.

## Key factors

- Owner directive (2026-10-10): PostgreSQL 18, `pgx`, sqlc and goose retained, backend-neutral
  repository interface in the store package. sqlc is pinned as a Go tool dependency in
  `tools/sqlc/go.mod`, invoked via `go tool -modfile tools/sqlc/go.mod sqlc`.
- SYSREQ-006/014/017/024 are technology-neutral by design; only the decision layer moves.
- One dialect only — a dual-backend design would double every query, migration, and test
  permanently; replacement avoids that tax entirely.
- Deterministic verification (ADR-006) now needs a disposable PostgreSQL per test run.

## Considered options

- Replace SQLite with PostgreSQL only.
- Dual backend (SQLite + PostgreSQL) behind one interface.
- Keep SQLite with a swap seam for later.
- dqlite (embedded replicated SQLite speaking Postgres wire).

## Decision Outcome

Chosen option: "PostgreSQL 18 with pgx, sqlc queries, goose migrations, behind a
backend-neutral repository interface", by owner directive; it fits the key factors — stateless
compute, reused operational HA/backup, a single dialect, and an interface seam that makes any
future engine change a package-internal affair.

### Positive Consequences

- Compute instances are stateless: restart, replace, and roll out without a volume; readiness
  degrades gracefully when the database is unreachable (SCEN-008).
- Backups, failover, and monitoring ride existing PostgreSQL operations.
- Multi-replica deployment is no longer architecturally forbidden (still out of v1 scope; the
  lease-based claim of SYSARCH-002 is the groundwork for that later increment).

### Negative Consequences

- Every deployment carries a database dependency — provisioning, credentials, migration rights —
  mitigated by goose-at-startup failing fast (SYSREQ-022 posture) and readiness gating.
- Local development needs a reachable PostgreSQL; the `justfile` exposes a `devdb` recipe so the
  loop stays one command (ADR-007).
- Test determinism now depends on a disposable database supplied via a test connection URL
  (ADR-006 updated accordingly).

## Pros and Cons of the Options

### PostgreSQL only (chosen)

- Good, because one dialect keeps query, migration, and CI cost at 1×.
- Good, because operational HA is delegated to infrastructure operators already run.
- Bad, because zero-dependency local startup is lost.

### Dual backend

- Bad, because sqlc's per-engine dialects turn every query into two handwritten twins forever.

### Keep SQLite with a seam

- Good, because it costs nothing today; bad, because the volume coupling and single-replica
  posture remain.

### dqlite

- Good, because it is Postgres-wire and replicated; bad, because CGO breaks the static-binary
  strategy of SYSARCH-005.

## Links

- Supersedes ADR-001; justifies SYSARCH-002.
- [pgx](https://github.com/jackc/pgx) · [sqlc](https://docs.sqlc.dev/) · [goose](https://github.com/pressly/goose)
