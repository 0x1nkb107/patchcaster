# Configuration

Every setting is a command line flag with an environment variable twin.
Precedence: **flags > environment > defaults**. Invalid configuration exits
non-zero before anything listens, naming every offending option at once.

## `patchcaster serve`

`serve` is the default command, so a bare `patchcaster` runs the service.

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

Durations use Go syntax (`90s`, `15m`, `4h`) and must be positive.
`--log-level` is one of `debug`, `info`, `warn` or `error`.

Any one of the signing secrets may sign a delivery, which is how the GitLab
signing token rotates without downtime: add the new secret, switch the hook
over to it, then drop the old one.

## `patchcaster migrate`

Applies the embedded database migrations and exits. It takes only
`--database-url` (`PATCHCASTER_DATABASE_URL`); see
[upgrades](operations.md#upgrades-and-migrations) for when to use it.

## Routing table

`--routing-file` names a YAML document mapping projects to channels;
[`contrib/routing.example.yaml`](../contrib/routing.example.yaml) is a
starting point:

```yaml
routes:
  - project: group/application   # GitLab project.full_path
    channel: "channel-id-here"   # Mattermost channel id, not its name
  - project: group/library
    channel: "channel-id-here"
```

Matching is exact and case-sensitive on `project.full_path`; there are no
wildcards and no catch-all. Many projects may share one channel; unmapped
projects are acknowledged and counted
(`patchcaster_deliveries_discarded_total{cause="unrouted"}`), never posted.

Every row needs both fields, and a project may appear only once; a table that
breaks either rule stops startup with an error naming each offending row. The
table is read at startup, so restart the service to apply changes.
