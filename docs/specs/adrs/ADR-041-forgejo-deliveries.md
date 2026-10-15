---
id: "ADR-041"
type: architecture_decision_record
name: "Forgejo deliveries are trusted by their signed body only"
description: "Forgejo signs only the body. PatchCaster verifies that signature, de-duplicates by a digest of the body, and caps age by the event time inside it."
status: proposed
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-007"
---

# ADR-041: Forgejo deliveries are trusted by their signed body only

## Context

GitLab signs deliveries with Standard Webhooks, which binds the delivery id, a timestamp and the body (ADR-002). Forgejo's webhook sender, checked in `services/webhook/shared/payloader.go` on 2026-10-15, signs only the body. It sends the hex HMAC-SHA256 as `X-Forgejo-Signature`, beside SHA-1 and compatibility headers. Its delivery id and event type travel in unsigned headers, and it sends no timestamp. v0.1 already judges an event's age by the time inside the signed body, not by delivery time (SYSREQ-013).

## Decision

- Verification checks `X-Forgejo-Signature` over the raw body against the connection's secrets, in constant time. All other signature headers are ignored.
- Duplicates are judged by a SHA-256 digest of the verified body, kept as long as SYSREQ-014 keeps delivery ids. A replay with a fresh `X-Forgejo-Delivery` header is still a duplicate.
- The event kind comes from `X-Forgejo-Event-Type`, but a body must have that kind's shape, or it is refused.
- The age cap uses the event time inside the body for every payload kind that has one. The adapter's documentation lists the fields.
- Payload kinds without an event time, such as pushes, are accepted on duplicate suppression alone.

## Consequences

- Good: an attacker who changes headers gains nothing, because only the signed body counts.
- Bad: a captured push delivery replayed after the duplicate retention would post again. Capturing one requires access to the delivery's TLS traffic or to a log of its body, so the risk is accepted rather than closed.
- Bad: if Forgejo ever adopts Standard Webhooks, the GitLab verification path can serve Forgejo too, and this decision should be revisited.
