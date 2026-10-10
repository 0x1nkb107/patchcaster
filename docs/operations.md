# Operations

Running PatchCaster once it is [installed](install.md): what to watch, how it
shuts down and upgrades, and what its delivery guarantees mean in practice.

## Probes and metrics

The internal listener (`--internal-addr`, `:9090` by default) serves:

| Endpoint | Answers |
| --- | --- |
| `GET /livez` | liveness; never depends on Mattermost |
| `GET /readyz` | readiness; store-backed, flips to 503 at SIGTERM |
| `GET /metrics` | Prometheus metrics, all prefixed `patchcaster_` |

The metrics follow a delivery from ingest to post:

| Metric | Labels | Counts |
| --- | --- | --- |
| `deliveries_received_total` | | webhook deliveries received |
| `deliveries_rejected_total` | `cause`: `signature`, `parse` | deliveries refused with 401/400 |
| `deliveries_suppressed_total` | | duplicates suppressed by webhook id |
| `deliveries_discarded_total` | `cause`: `unprocessed_kind`, `not_allowlisted`, `unlinked`, `expired`, `unrouted` | authentic deliveries dropped without a post |
| `state_applied_total` | `kind` | accepted deliveries applied to a status board |
| `notices_total` | `notice` | thread replies posted, by notice kind |
| `push_attribution_misses_total` | | pushes that matched no open merge request |
| `messages_posted_total` | | messages Mattermost accepted |
| `messages_skipped_total` | `cause`: `no_mapping`, `thread_closed`, `duplicate_open` | queued messages deliberately not posted |
| `messages_parked_total` | `cause`: `attempt_budget`, `thread_gone`, `render_failed`, `payload_undecodable` | messages given up on (see below) |
| `mattermost_attempts_total` | `outcome`: `sent`, `retryable`, `ratelimited`, `thread_gone`, `transport` | Mattermost API attempts |
| `mattermost_attempt_duration_seconds` | | histogram of attempt durations |

A rising `deliveries_discarded_total{cause="unprocessed_kind"}` usually means
a GitLab trigger is enabled that PatchCaster does not use; a missing trigger
shows up as silence instead (see [GitLab triggers](integrations/gitlab.md#triggers)).

## Drain and shutdown

SIGTERM stops acceptance and gives in-flight work `--drain-timeout`; set
`terminationGracePeriodSeconds` above it. Readiness turns 503 first, so the
instance leaves load balancer rotation before it stops listening.

## Upgrades and migrations

Migrations are embedded in the binary. Run `patchcaster migrate` before
rollout, or let startup apply them (single-replica v1). Both paths apply only
what is pending.

## Delivery guarantees

Delivery is at-least-once: a crash between Mattermost acceptance and
recording can duplicate a post; nothing is ever lost. Every accepted delivery
is first written to the `outbox` table, and a worker posts from there.

Failed Mattermost attempts retry with exponential backoff (1 s, doubling;
a rate limit's reset time wins when it is later). After 8 failed attempts the
message parks. Parked messages (`patchcaster_messages_parked_total`) surface
store reasons and stay in the `outbox` table for operator triage:

```sql
SELECT id, mr_ref, dest_channel, attempts, settled_reason, last_error, settled_at
FROM outbox
WHERE state = 'parked'
ORDER BY settled_at DESC;
```

## Scaling

v1 runs as a **single replica**; the queue uses lease-based claims
(`--claim-lease`) so more replicas remain a future option, not a schema
change.
