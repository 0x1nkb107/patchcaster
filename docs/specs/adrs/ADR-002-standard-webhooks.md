---
id: "ADR-002"
type: architecture_decision_record
name: "Standard Webhooks HMAC verification over static X-Gitlab-Token"
description: "Verify GitLab 19 signing tokens per the Standard Webhooks scheme instead of relying on the static secret token header."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-001"
---

# Architecture Decision: Standard Webhooks HMAC verification over static X-Gitlab-Token

## Context and problem statement

The bot's ingest endpoint is public. GitLab offers two authentication mechanisms for webhook
deliveries: the legacy static secret sent in `X-Gitlab-Token`, and — since GitLab 19.0
(generally available in 19.1) — signing tokens producing `webhook-signature` per the Standard
Webhooks specification. The decision fixes which the service trusts.

## Key factors

- A static header secret is replayable indefinitely and proves nothing about the body.
- The HMAC scheme binds identifier, timestamp, and exact payload bytes (SYSREQ-012, SYSREQ-013).
- v1 targets GitLab 19+ only, so the newer mechanism is available in scope.

## Considered options

- Verify `X-Gitlab-Token` only.
- Verify Standard Webhooks HMAC signatures only.
- Accept either mechanism.

## Decision Outcome

Chosen option: "Standard Webhooks HMAC verification only", because it is strictly stronger
(authenticity, integrity, bounded replay) and dual-mode acceptance doubles the attack surface for
no operational gain in a v1 scoped to GitLab 19+. Confirmed by the project owner on 2026-10-10:
security takes precedence, and the legacy static `X-Gitlab-Token` path is refused outright.

The HMAC key is the key the signing token carries. GitLab shows a token as `whsec_` followed by the base64 of a 32-byte key, and its documentation verifies by stripping the prefix and decoding the rest. v0.1.0 used the token's text as the key instead, so it refused GitLab's deliveries. Since 2026-10-17, PatchCaster decodes the token and refuses a secret in any other form (owner decision 2026-10-17).

### Positive Consequences

- One verification path; rejection logic (401) is unconditional and testable.
- Freshness window comes free with the scheme.

### Negative Consequences

- Self-managed GitLab 19.0 operators must have the `webhook_signing_token` feature flag on
  (it is on by default, removed in 19.1); document in deployment guide.

## Pros and Cons of the Options

### HMAC only

- Good, because forged or mutated deliveries are computationally infeasible.
- Good, because replay is time-bounded by design.
- Bad, because pre-19 GitLab cannot be served — accepted by the version constraint.

### Static token only

- Good, because it works on every GitLab version.
- Bad, because it is replayable and body-independent.

### Either accepted

- Good, because of maximum compatibility.
- Bad, because attackers get the weaker path; the strong mode is then decoration.

## Links

- Justifies SYSARCH-001; implements SYSREQ-012/013.
- [GitLab webhooks: signing tokens](https://docs.gitlab.com/user/project/integrations/webhooks/)
- [Standard Webhooks specification](https://www.standardwebhooks.com/)