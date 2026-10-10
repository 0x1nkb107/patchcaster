# PatchCaster

Forwards GitLab (19.0+) merge request webhook events into Mattermost channels —
one thread per merge request: `open` posts the thread root, every later
allowlisted action posts a reply. The normative behavior lives in the SARA
graph under [`docs/`](docs/README.md); this README is the operator's quickstart.

## Build & run

The toolchain (sqlc, golangci-lint) is pinned as Go tool modules under
`tools/` — no ambient installs are needed beyond Go itself; `just` orchestrates
the tasks.

```sh
just test            # offline unit suite
just devdb           # PostgreSQL 18 on 127.0.0.1:5433 (podman)
just migrate         # apply embedded schema
just run             # serve with the example routing table (needs env vars below)
just lint            # vet + pinned golangci-lint
just sqlc            # regenerate query code after touching db/
just container       # OCI image (distroless, CGO-free)
```

## Configuration

Precedence: **flags > environment > defaults**. Invalid configuration exits
non-zero before anything listens.

| Flag | Env | Default |
| --- | --- | --- |
| `--addr` | `PATCHCASTER_ADDR` | `:8080` |
| `--internal-addr` | `PATCHCASTER_INTERNAL_ADDR` | `:9090` |
| `--database-url` | `PATCHCASTER_DATABASE_URL` | *required* |
| `--mattermost-url` | `PATCHCASTER_MATTERMOST_URL` | *required* |
| `--mattermost-token` | `PATCHCASTER_MATTERMOST_TOKEN` | *required* (bot account token) |
| `--signing-secret` (repeatable) | `PATCHCASTER_SIGNING_SECRETS` (comma-separated) | *required* |
| `--routing-file` | `PATCHCASTER_ROUTING_FILE` | *required* |
| `--drain-timeout` | `PATCHCASTER_DRAIN_TIMEOUT` | `30s` |
| `--event-max-age` | `PATCHCASTER_EVENT_MAX_AGE` | `4h` |
| `--seen-retention` | `PATCHCASTER_SEEN_RETENTION` | `24h` |
| `--claim-lease` | `PATCHCASTER_CLAIM_LEASE` | `60s` |
| `--log-level` | `PATCHCASTER_LOG_LEVEL` | `info` |

`--seen-retention` must exceed `--event-max-age` — that inequality is what
closes the replay loop (dedupe outlives the freshness cap).

### GitLab side

Use a **Standard Webhooks**-capable hook (system, group, or project) with the
signing token enabled. PatchCaster verifies only the `webhook-signature` HMAC
(`v1,{base64}` over `{webhook-id}.{timestamp}.{body}`, ADR-002); the legacy
`X-Gitlab-Token` is never trusted. The endpoint is `POST /webhook`.

### Routing table

See [`contrib/routing.example.yaml`](contrib/routing.example.yaml): exact,
case-sensitive `project.full_path` → Mattermost channel id. Many projects may
share one channel; unmapped projects are acknowledged and counted, never
posted.

## Operations

* **Probes/metrics** on the internal listener: `GET /livez`, `GET /readyz`
  (store-backed, flips to 503 at SIGTERM), `GET /metrics` (`patchcaster_*`
  funnel counters). Liveness never depends on Mattermost.
* **Drain**: SIGTERM stops acceptance and gives in-flight work
  `--drain-timeout`; set `terminationGracePeriodSeconds` above it.
* **Migrations** are embedded in the binary: run `patchcaster migrate` before
  rollout, or let startup apply them (single-replica v1).
* **Delivery semantics** are at-least-once: a crash between Mattermost
  acceptance and recording can duplicate a post; nothing is ever lost. Parked
  messages (`patchcaster_messages_parked_total`) surface store reasons and stay
  in the `outbox` table for operator triage.
* v1 runs as a **single replica**; the queue uses lease-based claims so more
  replicas remain a future option, not a schema change.

## Testing

`go test ./...` is fully offline; store contract tests run against a real
PostgreSQL when `PATCHCASTER_TEST_DATABASE_URL` points at a *disposable*
database (CI provides one).
