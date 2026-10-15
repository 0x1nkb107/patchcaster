---
id: "SYSARCH-007"
type: system_architecture
name: "Forge and chat adapters"
description: "Each forge and each chat platform sits behind an adapter: forges verify and translate webhooks, chat platforms deliver, edit and receive commands."
status: proposed
satisfies:
  - "SYSREQ-044"
  - "SYSREQ-045"
  - "SYSREQ-052"
  - "SYSREQ-056"
  - "SYSREQ-057"
  - "SYSREQ-058"
  - "SYSREQ-059"
  - "SYSREQ-060"
  - "SYSREQ-061"
  - "SYSREQ-063"
---

# System Architecture: Forge and chat adapters

Normative once accepted: everything specific to a forge or a chat platform lives in that platform's adapter. The core sees only neutral events, sinks and capabilities (ADR-036).

## Technical Strategy

A forge adapter verifies a delivery, de-duplicates it, and translates it into neutral merge request events. Where the connection has API access, it also registers hooks, reads required approvals and runs the OAuth flow for identity links. A chat adapter declares its capabilities: threads, edits, mentions, its native format and its command syntax. It creates roots, replies, edits roots, and turns chat commands into linking requests. GitLab and Mattermost become the first adapters, carved out of today's `event`, `ingest` and `mattermost` packages. Forgejo (ADR-041) and Matrix (ADR-040) follow, and the second implementation of each kind shows where the seam belongs.

## Static View (Structure)

- **Forge adapter**: `Verify(raw body, headers, secrets)`, `Translate(body) -> []Event`, and optional `RegisterHook`, `RequiredApprovals`, `AuthorizeIdentity`.
- **Chat adapter**: `Capabilities`, `CreateRoot`, `Reply`, `EditRoot`, `Commands`.
- **GitLab**: Standard Webhooks verification as today (ADR-002).
- **Forgejo**: body HMAC, body-digest de-duplication, body event time (ADR-041).
- **Mattermost**: the bot account REST API as today (SYSREQ-004), plus a slash-command endpoint on the webhook listener and the registration of that command in the bot's teams (SYSREQ-063).
- **Matrix**: mautrix-go built with goolm. One long-running session per connection runs the sync loop, the encryption machine and the command handling. It holds the connection's ownership lock (SYSREQ-061).

## Dynamic View (Behavior)

Inbound, a forge delivery arrives at its connection's endpoint. The adapter verifies, de-duplicates and translates it, and the core routes it (SYSARCH-008). Outbound, the delivery worker hands each queued message to the sink's chat adapter. Commands arrive as Mattermost slash-command requests on the webhook listener, or as messages in a Matrix session's sync stream. The adapter passes them to the identity service (SYSARCH-006).

## Notes

Proposed on 2026-10-15. On acceptance this generalizes SYSARCH-001 and SYSARCH-003.
