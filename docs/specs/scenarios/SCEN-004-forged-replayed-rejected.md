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

The channel is protected by cryptographic verification: unsigned and wrongly signed deliveries
are refused outright, while authentic-but-expired and duplicate deliveries are acknowledged and
discarded — never posted, and never charged against GitLab's webhook failure counters.

## Initial State

- Signing tokens for configured sources are loaded; the seen-identifier store (24 h retention,
  SYSREQ-014) and the event-age cap (4 h, SYSREQ-013) are active.

## Trigger

Any of: a delivery missing `webhook-signature`; one signed with the wrong secret; a re-delivery
of an already accepted `webhook-id` (GitLab's own retry); a verified delivery whose event
timestamp is older than 4 h.

## Step-by-Step Flow

1. The service recomputes the HMAC-SHA256 over `webhook-id`, timestamp, and raw body bytes.
2. Failed or missing signatures are refused with `401` before any content decision.
3. Verified deliveries with a seen identifier are acknowledged as duplicates.
4. Verified deliveries whose event is older than the cap are acknowledged and discarded.

## Expected Outcome

- Forgeries answer `401`; nothing is queued, nothing is posted.
- Duplicates and expired events answer `2xx` with zero posts — this deliberately keeps GitLab's
  four-failures-disable threshold (SYSREQ-013 rationale) away from honest traffic.
- Rejections, suppressions, and expiries are visible in metrics without leaking secrets.

## Exceptions & Edge Cases

- Replay of a captured delivery is bounded by two interlocking walls: inside the 24 h retention
  the seen identifier suppresses it; after retention the event is necessarily older than the
  4 h cap and is discarded as expired.
- GitLab retries of already-accepted messages are the expected duplicate traffic — the dedupe
  path exists for them, not against them.
