---
id: "SYSREQ-012"
type: system_requirement
name: "Standard Webhooks signature verification"
description: "Every delivery must carry a valid GitLab 19 webhook signature."
specification: "The service SHALL reject with HTTP 401 every delivery whose webhook-signature header does not carry a valid HMAC-SHA256 signature over the webhook identifier, timestamp, and payload as specified by Standard Webhooks."
derives_from:
  - "SCEN-004"
---

# System Requirement: Standard Webhooks signature verification

## Requirement Specification

> The service SHALL reject with HTTP 401 every delivery whose webhook-signature header does not carry a valid HMAC-SHA256 signature over the webhook identifier, timestamp, and payload as specified by Standard Webhooks.

## Rationale

GitLab's static `X-Gitlab-Token` is a replayable plaintext secret. GitLab 19's signing token
implements the Standard Webhooks scheme, binding the delivery's identifier, timestamp, and exact
body bytes — a forged or mutated delivery becomes computationally infeasible (ADR-002; see
[GitLab webhook signing](https://docs.gitlab.com/user/project/integrations/webhooks/) and the
[Standard Webhooks](https://www.standardwebhooks.com/) specification).

## Acceptance Criteria

- A delivery whose recomputed HMAC-SHA256 over the signed content (`id`, timestamp, raw body)
  matches any configured signing secret proceeds.
- Missing, malformed, or mismatched `webhook-signature` answers `401` before the body is parsed
  and with zero side effects.
- The `X-Gitlab-Token` header is never trusted (ADR-002): a request with a matching static token
  but no valid signature is refused exactly like any other unverified delivery.
- Signature recomputation uses the raw request bytes, never a re-serialized body.

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests sign fixtures with the same construction, include the Standard Webhooks
  test vectors, and cover each failure mode separately — including a valid `X-Gitlab-Token`
  without a signature, which must be refused.

## Notes

Verification runs as middleware so every later stage can trust the payload's origin.
