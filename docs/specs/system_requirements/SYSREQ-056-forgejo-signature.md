---
id: "SYSREQ-056"
type: system_requirement
name: "Forgejo signature verification"
description: "Every Forgejo delivery must carry a valid HMAC-SHA256 signature over its body."
specification: "The service SHALL reject with HTTP 401 every Forgejo delivery whose X-Forgejo-Signature header is not the hex-encoded HMAC-SHA256 of the raw request body under one of the connection's secrets."
status: proposed
derives_from:
  - "SCEN-027"
---

# System Requirement: Forgejo signature verification

## Requirement Specification

> The service SHALL reject with HTTP 401 every Forgejo delivery whose X-Forgejo-Signature header is not the hex-encoded HMAC-SHA256 of the raw request body under one of the connection's secrets.

## Rationale

Forgejo does not implement Standard Webhooks. Its webhook sender signs only the body, with HMAC-SHA256, and sends the hex digest as `X-Forgejo-Signature`. It also sends SHA-1 and Gitea, Gogs and GitHub compatible headers, which PatchCaster ignores (`services/webhook/shared/payloader.go` in Forgejo's source, checked 2026-10-15).

## Acceptance Criteria

- A matching signature under any of the connection's secrets proceeds.
- A missing, malformed or mismatched signature answers `401` before the body is parsed, with no side effect.
- The comparison runs in constant time over the raw request bytes.
- The SHA-1 signature header is never accepted in place of the SHA-256 one.

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests sign fixtures the way Forgejo's sender does and cover each failure mode, including a valid SHA-1 signature with a missing SHA-256 one.
