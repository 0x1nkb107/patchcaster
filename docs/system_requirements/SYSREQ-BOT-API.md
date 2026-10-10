---
id: "SYSREQ-004"
type: system_requirement
name: "Delivery through the bot account API"
description: "All messages are posted via the Mattermost bot account REST API."
specification: "The service SHALL deliver every message through the Mattermost REST API v4 posts endpoints, authenticated with a bot account token."
derives_from:
  - "SCEN-001"
---

# System Requirement: Delivery through the bot account API

## Requirement Specification

> The service SHALL deliver every message through the Mattermost REST API v4 posts endpoints, authenticated with a bot account token.

## Rationale

Only the bot account API can address arbitrary channels by identifier and nest replies with
`root_id`; incoming webhooks are fixed per channel and cannot thread reliably against a stored
root. One credential and one surface keep the trust model simple.

## Acceptance Criteria

- Every message is sent as `POST {site-url}/api/v4/posts` with `Authorization: Bearer <bot-token>`.
- The token is provided via configuration (SYSREQ-022) and never appears in a URL or query string.
- No code path posts to an incoming-webhook URL.

## Verification Plan

- **Method**: Test.
- **Procedure**: `httptest` Mattermost mock asserts method, path, and headers on every delivery
  path (root, reply, retry).

## Notes

The bot account being a member of target channels is a deployment precondition (UC-002).
