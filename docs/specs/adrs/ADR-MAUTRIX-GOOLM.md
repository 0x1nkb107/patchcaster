---
id: "ADR-040"
type: architecture_decision_record
name: "mautrix-go with its pure-Go goolm crypto for Matrix"
description: "The Matrix adapter uses mautrix-go built with the goolm tag, keeps its encryption state in PostgreSQL, and holds one lock per device."
status: proposed
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-007"
---

# ADR-040: mautrix-go with its pure-Go goolm crypto for Matrix

## Context

Encrypted Matrix rooms are required from the start (2026-10-15). The owner pointed at mautrix-go, whose `cryptohelper` handles the encryption machine, and at a write-up of a simple encrypted bot built with it. mautrix-go v0.31.0, checked on 2026-10-15, uses libolm through cgo by default and its own pure-Go implementation, goolm, when built with `-tags goolm`. PatchCaster's build is CGO-free (SYSARCH-005), and libolm is deprecated upstream. The owner accepted goolm, although no independent audit of it is known.

## Decision

- The Matrix adapter uses `maunium.net/go/mautrix`, licensed MPL-2.0, which is compatible with AGPL-3.0. Every build, release and CI run uses `-tags goolm`. With CGO off, a build without the tag fails, so the build contract enforces the tag.
- The crypto store lives in PostgreSQL. `cryptohelper` accepts a `dbutil` database, and wrapping PatchCaster's own pool is to be confirmed during implementation.
- The pickle key is random and sealed by the secret box (ADR-035). The recovery key is sealed too, and is used once per device to cross-sign it.
- One long-running session per Matrix connection runs the sync loop. It tracks room members and their devices, shares room keys and reads linking commands.
- A PostgreSQL advisory lock per connection, held on a dedicated connection, ensures one owner per device (SYSREQ-061).
- End-to-end tests run a real homeserver in a container, such as Synapse, and a second mautrix-go client that must decrypt what the bot sent.

## Consequences

- Good: encrypted rooms work, and the binary stays static and CGO-free.
- Bad: goolm has no known independent audit. A future audit, or a move to another implementation, stays possible behind the adapter.
- Bad: mautrix-go brings `zerolog` and `go.mau.fi/util` along, so its logs need a bridge to `log/slog` (ADR-004).
- Bad: the Matrix adapter is the first component that must run continuously, not just per message.
