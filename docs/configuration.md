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
| `--management-addr` | `PATCHCASTER_MANAGEMENT_ADDR` | `:8081` |
| `--interface-url` | `PATCHCASTER_INTERFACE_URL` | *required* (see [interface URL](#interface-url)) |
| `--public-url` | `PATCHCASTER_PUBLIC_URL` | *required* (see [public URL](#public-url)) |
| `--database-url` | `PATCHCASTER_DATABASE_URL` | *required* |
| `--encryption-key` | `PATCHCASTER_ENCRYPTION_KEY` | *required* (see [encryption key](#encryption-key)) |
| `--previous-encryption-key` (repeatable) | `PATCHCASTER_PREVIOUS_ENCRYPTION_KEYS` (comma-separated) | none |
| `--drain-timeout` | `PATCHCASTER_DRAIN_TIMEOUT` | `30s` |
| `--event-max-age` | `PATCHCASTER_EVENT_MAX_AGE` | `4h` |
| `--seen-retention` | `PATCHCASTER_SEEN_RETENTION` | `24h` |
| `--claim-lease` | `PATCHCASTER_CLAIM_LEASE` | `60s` |
| `--poll-interval` | `PATCHCASTER_POLL_INTERVAL` | `10s` |
| `--log-level` | `PATCHCASTER_LOG_LEVEL` | `info` |

`--seen-retention` must exceed `--event-max-age` — that inequality is what
closes the replay loop (dedupe outlives the freshness cap).

A new message wakes the delivery worker at once through PostgreSQL
notifications. `--poll-interval` bounds how late the worker picks up retries
that come due, messages whose worker died mid-delivery, and messages whose
wake-up was missed. It is also how quickly messages go out when wake-ups are
unavailable (see [database connection](operations.md#database-connection)).

Durations use Go syntax (`90s`, `15m`, `4h`) and must be positive.
`--log-level` is one of `debug`, `info`, `warn` or `error`.

Flags and environment variables hold only how PatchCaster starts and operates. What it connects to and routes, its connections, teams, sources, sinks and routes, lives in its database, and Instance Admins manage it through the management API. See [connections](operations.md#connections) and [teams and routes](operations.md#teams-and-routes). A change there applies to the next event, without a restart.

## Interface URL

`--interface-url` is the address at which people reach the management listener, such as `https://patchcaster.example.org`. PatchCaster builds the identity provider's redirect address and the setup link from it, so it must be the address browsers use, also behind a reverse proxy. It names only a scheme, a host and an optional port: the interface answers at the root of its listener. With `https`, the session cookie is marked `Secure`. Use plain `http` only for a local instance.

## Public URL

`--public-url` is the address at which the forges reach the webhook listener, such as `https://hooks.patchcaster.example.org`. PatchCaster builds each forge connection's webhook endpoint from it, so it must be the address the forges use, also behind a reverse proxy. It may carry a path, when the proxy forwards a prefix to the listener.

## Encryption key

PatchCaster seals the secrets it stores in the database with the instance encryption key: 32 random bytes, base64-encoded. Generate one with `openssl rand -base64 32` and guard it like the database credentials. Prefer the environment variable over the flag, because flags show up in process listings.

At its first start PatchCaster stores a sealed check value, and every later start must open it. A start with another key therefore fails before it touches any secret, with a message that names the problem and never the key. Losing the key loses the stored secrets. To change the key, see [rotating the encryption key](operations.md#rotating-the-encryption-key).

## `patchcaster migrate`

Applies the embedded database migrations and exits. It takes only
`--database-url` (`PATCHCASTER_DATABASE_URL`); see
[upgrades](operations.md#upgrades-and-migrations) for when to use it.

## `patchcaster bootstrap`

Prints the one-time setup link of a new instance and exits. It takes `--database-url` and `--interface-url`, with the same environment variables as `serve`. The link works once, for one hour, and only while no Instance Admin exists. See [first setup](operations.md#first-setup).

## `patchcaster admin grant`

Makes a person an Instance Admin and exits, whether or not they ever logged in. It takes `--database-url` (`PATCHCASTER_DATABASE_URL`), and `--issuer` and `--subject`, which name the person exactly as their ID tokens do. See [restoring Instance Admin access](operations.md#restoring-instance-admin-access).

## `patchcaster secrets reseal`

Seals every stored secret again with `--encryption-key` and exits. It takes `--database-url`, `--encryption-key` and `--previous-encryption-key`, with the same environment variables as `serve`. See [rotating the encryption key](operations.md#rotating-the-encryption-key) for when to use it.
