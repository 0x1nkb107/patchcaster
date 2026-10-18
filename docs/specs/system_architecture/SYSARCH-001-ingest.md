---
id: "SYSARCH-001"
type: system_architecture
name: "Webhook ingest pipeline"
description: "Inline verify, filter, and route inside the request handler; hand off to the queue."
satisfies:
  - "SYSREQ-001"
  - "SYSREQ-008"
  - "SYSREQ-010"
  - "SYSREQ-011"
  - "SYSREQ-012"
  - "SYSREQ-013"
  - "SYSREQ-014"
  - "SYSREQ-015"
---

# System Architecture: Webhook ingest pipeline

Normative: the ingest path verifies, filters, and routes inline within one request handler before
handing the event to the delivery queue.

## Technical Strategy

Single `net/http` handler chain on the standard library `http.ServeMux` (Go 1.22+ method
patterns). Middleware order is part of the design: body-size limit and raw-body capture first
(the HMAC covers exact bytes), then signature verification (SYSREQ-012), then identifier dedupe
on headers alone — the check only; the seen-mark itself rides the enqueue transaction
(SYSREQ-014) — then parse; the post-parse expiry cap (SYSREQ-013, event time
lives in the signed body) precedes the action allowlist (SYSREQ-008), then routing
(SYSREQ-010/011, SYSREQ-046), then outbox enqueue — persisting the resolved sink and channel
with each row (SYSREQ-024) — then the `2xx` write. No framework: the surface is a
dozen routes wide and the standard library removes a dependency boundary.

## Static View (Structure)

Components: `WebhookHandler`, `Signer` (verifier), `Allowlist`, `Keyring` (the signing tokens of
each forge connection), `Store` (SYSARCH-002 interface: `SeenWebhook`, `Destinations`, `Enqueue`).

Interfaces are narrow and local: the handler depends on `Store` and `Keyring` only, so ingest
tests run against fakes without a database or network.

Generalized on 2026-10-18 by SYSARCH-007 and SYSARCH-008: every forge connection has an endpoint of its own, `POST /webhooks/{connection}`, verified with that connection's signing tokens, and the routes stored in the database resolve the sinks of each event.

## Dynamic View (Behavior)

One request per delivery, synchronous through verification and enqueue; asynchronous from there.
Failure paths exit the chain early: auth failures `401` before parse; unrouted projects and
non-allowlisted actions `2xx` before enqueue. Every path ends in exactly one metric increment and
structured log records carrying `webhook-id` (SYSREQ-019, SYSREQ-020).
