---
id: "SCEN-009"
type: scenario
name: "Operators observe delivery health"
description: "Metrics and logs let an operator answer how many events arrived, were posted, or were rejected."
refines:
  - "UC-003"
---

# Scenario: Operators observe delivery health

## Overview

An operator answers "how many events arrived, were rejected, were suppressed, were posted, and
why?" from metrics and logs alone.

## Initial State

- Traffic has flowed, including at least one rejection and one duplicate suppression.

## Trigger

The operator scrapes the metrics endpoint and searches logs by a known `webhook-id`.

## Step-by-Step Flow

1. Counters for received, rejected, suppressed, posted, and delivery attempts render in Prometheus text format.
2. Every log line related to one delivery carries that delivery's `webhook-id`.

## Expected Outcome

- Counter deltas match the traffic actually sent within a test run.
- A log grep by `webhook-id` yields the full path (verify, route, queue, post attempts) for exactly one delivery.
- No bot token or signing secret appears anywhere in the output, including failure paths.

## Exceptions & Edge Cases

- Rejections and discards are counted by cause (signature, expired, duplicate) so 401 storms
  and expiry floods are diagnosable.
