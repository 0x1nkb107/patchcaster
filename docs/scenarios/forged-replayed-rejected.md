---
id: "SCEN-004"
type: scenario
name: "Forged and replayed deliveries are rejected"
description: "Unsigned, wrongly signed, stale, or duplicate deliveries never reach the channel."
refines:
  - "UC-001"
---

# Scenario: Forged and replayed deliveries are rejected

## Overview

The channel is protected by cryptographic verification: unsigned, wrongly signed, stale, and
replayed deliveries never become posts.

## Initial State

- Signing tokens for configured sources are loaded; freshness window and seen-identifier store are active.

## Trigger

Any of: a delivery missing `webhook-signature`; one signed with the wrong secret; one whose
`webhook-timestamp` is older than the freshness window; a re-delivery of an already accepted
`webhook-id` inside the window.

## Step-by-Step Flow

1. The service recomputes the HMAC-SHA256 over `webhook-id`, timestamp, and raw body.
2. Mismatched, missing, or stale deliveries are refused before any routing decision.
3. Exact duplicates of accepted identifiers are acknowledged without re-processing.

## Expected Outcome

- Signature and freshness failures answer 401; nothing is queued, nothing is posted.
- Duplicate identifiers answer 2xx but produce no second post (GitLab re-delivery is benign).
- Rejections are visible in metrics without leaking secrets into logs.

## Exceptions & Edge Cases

- Future-dated timestamps outside the window are refused the same way (bound clock-skew abuse).
- A legitimate delivery replayed *after* GitLab's own retry period is indistinguishable from a
  duplicate; suppression is time-bounded (SYSREQ-014).
