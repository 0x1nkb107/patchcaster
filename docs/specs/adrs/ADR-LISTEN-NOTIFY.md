---
id: "ADR-019"
type: architecture_decision_record
name: "PostgreSQL LISTEN/NOTIFY wakes the delivery worker"
description: "Ingest notifies on commit for every enqueued row so the worker claims at once; polling at a longer interval remains the fallback for retries, expired leases, and missed notifications."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-002"
  - "SYSARCH-003"
  - "SYSARCH-004"
---

# Architecture Decision: PostgreSQL LISTEN/NOTIFY wakes the delivery worker

## Context and problem statement

The delivery worker found new outbox rows only by polling once per second, so every message
waited up to a second before its first attempt, and an idle service still ran a claim query
every second. The owner decided (2026-10-11) that ingest wakes the worker directly through
PostgreSQL `LISTEN`/`NOTIFY` and that the poll interval grows to 10 seconds, configurable.

## Key factors

- Owner directive (2026-10-11): wake-ups via `LISTEN`/`NOTIFY` plus a longer, configurable poll.
- `NOTIFY` is transactional: PostgreSQL delivers it only when the sending transaction commits and
  drops it on rollback. Sent from the accept transaction of SYSREQ-017, it can never announce a
  row that is not yet visible, and it adds no state of its own.
- Retries coming due (`next_attempt_at`) and expired claim leases have no committing event to
  announce them, so polling cannot go away; it becomes the fallback.
- Notifications are lost while no session listens, for example during a reconnect. Each new
  session must therefore start with a claim pass.
- A transaction-mode connection pooler (PgBouncer and similar) accepts `LISTEN` but never forwards
  notifications. Without a check the worker would silently fall back to the slower poll.
- No new dependency: pgx, already the driver (ADR-010), exposes notifications on a plain
  connection, and cenkalti/backoff (ADR-008) can pace reconnects.

## Considered options

- Keep polling every second; no wake-up.
- `LISTEN`/`NOTIFY` wake-ups plus a longer poll (chosen).
- An in-process channel from ingest to the worker.
- Adopt a job-queue library that has its own notifier (River).

## Decision Outcome

Chosen option: "`LISTEN`/`NOTIFY` wake-ups plus a longer poll", by owner directive, with these
binding constraints:

- The accept transaction calls `pg_notify` on the `patchcaster_outbox` channel whenever it
  enqueues a row. The payload is empty: a wake-up means "claim again", never "claim this row", so
  the claim order of SYSARCH-002 stays the only authority on what runs next.
- The listener holds its own connection outside the pool, because a listening connection stays
  checked out for as long as it listens.
- A session counts as listening only after a probe notification it sent to itself has come back.
  That round trip is what exposes a pooler that swallows notifications.
- Every session that reaches listening wakes the worker once, to cover rows committed while no
  session listened. Lost sessions reconnect with exponential backoff.
- Wake-ups coalesce into at most one pending signal, so a burst of commits costs one extra claim
  pass, not one per row.
- The poll interval (`--poll-interval`, default 10 s) bounds how late retries, expired leases, and
  missed notifications are picked up.
- A gauge reports whether a confirmed session is listening, so a pooler or connection problem
  shows up in metrics instead of only as slower delivery.

### Positive Consequences

- A new message is claimed as soon as its row commits instead of up to a poll interval later.
- An idle service runs one claim pass every 10 s instead of every second.
- With several replicas every listening worker wakes and `SKIP LOCKED` picks one claimer, so the
  design carries over to the multi-replica increment unchanged.

### Negative Consequences

- Each replica holds one more database connection for the listener.
- Behind a transaction-mode pooler wake-ups never work; delivery still happens, at the poll
  interval, and the gauge stays at zero.
- PostgreSQL serializes the commits of all transactions that sent `NOTIFY` with a server-wide
  lock, to keep the notification queue in commit order. At webhook volumes this is negligible.

## Pros and Cons of the Options

### Keep polling every second

- Good, because it needs no code and no extra connection.
- Bad, because latency stays bounded by the poll and idle load stays at one query per second.

### LISTEN/NOTIFY plus a longer poll (chosen)

- Good, because the wake-up is transactional with the enqueue and works across replicas.
- Bad, because it adds a connection, a supervised session, and a pooler caveat.

### In-process channel

- Good, because it is the simplest wake-up: no connection, no reconnects.
- Bad, because it only wakes the worker in the replica that accepted the webhook, which breaks
  as soon as more than one replica runs.

### River

- Good, because its notifier, leader election, and maintenance are maintained elsewhere.
- Bad, because the queue's ordering per merge request would need River Pro, and the outbox would
  move to River's schema and migrations.

## Links

- Justifies SYSARCH-002, SYSARCH-003, and SYSARCH-004; builds on ADR-008 and ADR-010.
- [PostgreSQL NOTIFY](https://www.postgresql.org/docs/18/sql-notify.html) ·
  [PostgreSQL LISTEN](https://www.postgresql.org/docs/18/sql-listen.html) ·
  [PgBouncer feature map](https://www.pgbouncer.org/features.html)
