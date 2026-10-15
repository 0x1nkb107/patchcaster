---
id: "ADR-008"
type: architecture_decision_record
name: "cenkalti/backoff for retry scheduling"
description: "Retry intervals for Mattermost deliveries are computed by github.com/cenkalti/backoff."
status: accepted
deciders:
  - "Project owner"
justifies:
  - "SYSARCH-003"
---

# Architecture Decision: cenkalti/backoff for retry scheduling

## Context and problem statement

SYSREQ-016 fixes a normative retry schedule: exponential backoff from 1 s, doubling, capped at
10 minutes, with an 8-attempt budget and a `RateLimit-Reset` override. The decision is which
library computes that schedule, per the owner directive of 2026-10-10.

## Key factors

- Owner directive: use `github.com/cenkalti/backoff`.
- Attempts are driven by the durable outbox (SYSREQ-017): the *clock and persistence* of
  `next_attempt_at` belong to the queue, not to an in-process retry loop.
- The `RateLimit-Reset` deferral may only lengthen an interval, never shorten it.

## Considered options

- `cenkalti/backoff` as an interval generator.
- Hand-rolled exponential schedule.
- Other retry helpers (hashicorp/go-retryablehttp, retry-go).

## Decision Outcome

Chosen option: "`cenkalti/backoff`", by owner directive; it fits anyway, with binding
implementation constraints. The library (`v5` module) is used as an **interval generator**
(`ExponentialBackOff{InitialInterval: 1s, Multiplier: 2, MaxInterval: 10m}` plus the
`NextBackOff()` sequence), not as a `Retry`-loop driver, because each scheduled attempt must be
persisted in the durable queue between tries. `RandomizationFactor` is set to `0` so the ladder is
the exact 1s, 2s, 4s … 10min schedule — precisely testable per attempt. The 15-minute
`MaxElapsedTime` abort is structurally impossible in `v5`: the field was removed from the backoff
struct (it only governed the unused `Retry` driver), so the 8-attempt budget (~45 min worst-case
horizon) cannot be cut short; the worker builds a fresh generator and steps it once per recorded
failure, making the deferral a pure function of the attempt count.

### Positive Consequences

- Battle-tested timing code; jitter options available without hand-rolling them.
- The SYSREQ-016 constants map one-to-one onto named library fields — no folklore numbers.

### Negative Consequences

- One more small dependency; contained inside the delivery worker, matching the narrow
  `MattermostClient` boundary of SYSARCH-003.
- `RateLimit-Reset` is outside the library's model: the worker schedules the later of
  `NextBackOff()` and the reset instant — stated here so a future reader does not mistake the
  library as the sole authority on timing.

## Pros and Cons of the Options

### cenkalti/backoff

- Good, because context-awareness and exponential semantics are already tested for us.
- Good, because interval generation composes with the durable queue cleanly.
- Bad, because its `Retry` driver idiom fights the outbox pattern — deliberately not used.

### Hand-rolled schedule

- Good, because it is one fewer dependency.
- Bad, because timer edge-case coverage becomes our test burden for no gain.

## Links

- Justifies SYSARCH-003; implements SYSREQ-016.
- [cenkalti/backoff](https://github.com/cenkalti/backoff)
