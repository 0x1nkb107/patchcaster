# Operations

Running PatchCaster once it is [installed](install.md): what to watch, how it
shuts down and upgrades, and what its delivery guarantees mean in practice.

## Web interface and management API

The management listener (`--management-addr`, `:8081` by default) serves the web interface at the interface URL, the management API under `/api/v1/`, and the API's OpenAPI description at `GET /api/v1/openapi.yaml`. It answers nothing else, and the webhook and internal listeners never answer interface or API paths, so each listener can be exposed differently: the webhook listener to the forges, the management listener only to the organization's people and automation. Errors are RFC 9457 problem details (`application/problem+json`). A validation error lists each field at fault by its path, such as `allowed_groups[0]`. Lists page with `limit` and the `next_cursor` of the previous page. Every change sends the `ETag` of what it read in `If-Match`, and fails with `412 Precondition Failed` when the resource changed meanwhile.

The web interface covers the first setup, the login and API tokens so far. Everything else goes through the API, which the interface uses too. The interface runs only its own scripts, and no other site may show it in a frame. Every page links to the licences of the third-party packages it contains.

The management listener also serves the login. People log in through the organization's OpenID Connect provider, and PatchCaster keeps no passwords. A session ends after one hour without activity, twelve hours after login at the latest, or at logout (`DELETE /api/v1/session`). The API refuses any change that a browser sends from another site.

## First setup

A new instance has no identity provider and no administrator yet:

1. Register PatchCaster as a confidential client at the identity provider, with the authorization code flow and the redirect address `<interface URL>/auth/callback`.
2. Run `patchcaster bootstrap`. It prints a setup link of the form `<interface URL>/setup#<token>`, which works once, for one hour.
3. Open the setup link in the browser that will log in, and enter the provider's issuer, the client ID and the client secret. PatchCaster checks that a provider answers at the issuer, seals the client secret, and sends the browser on to the login. Without the interface, send the token from the link and the same values to `POST /api/v1/setup` from that browser. It answers with the login address.
4. Log in in the same browser. You become the first administrator, and the setup link stops working.

Everyone who logs in later starts with the role User. To admit only some of the people the provider knows, name the ID token claim that lists a person's groups (`groups_claim`) and the groups that may log in (`allowed_groups`), at setup or later through `PUT /api/v1/settings/login`. A refused login tells the person why.

## Connections

A connection holds what PatchCaster acts with on one platform: GitLab as a forge, Mattermost as a chat platform. Administrators manage them at `/api/v1/connections`. A connection's credential is checked against its platform before it is stored, and the connection shows the account the credential acts as. A token the platform refuses is not stored, and the answer names the platform's reason. The credential is stored encrypted and is never shown again: send a new one to replace it.

- A **Mattermost** connection needs the bot account's token.
- A **GitLab** connection needs an access token only for what PatchCaster does through the GitLab API. Without one, register the hooks by hand at the connection's `webhook_url`. Then generate a signing token in each hook's form and add it to the connection under `/api/v1/connections/{id}/signing-tokens`, exactly as GitLab shows it. A connection holds several signing tokens at once, so a token rotates without downtime: add the new one, switch the hook over, then remove the old one.

A connection that sources or sinks use cannot be removed. A changed credential or signing token applies to the next delivery, without a restart.

## Teams and routes

Teams decide who may route what. Administrators create teams at `/api/v1/teams`, name each team's operators, and give it sources and sinks. A source or sink may belong to several teams, such as a shared release channel.

- A **source** selects the repositories of a forge connection, at `/api/v1/sources`: the whole instance, a group with everything below it, or one repository.
- A **sink** names a channel of a chat connection, at `/api/v1/sinks`. PatchCaster checks that the connection's bot can post there before it stores the sink, and names the reason when it cannot, such as a bot that is not a member of the channel.
- A **route**, at `/api/v1/routes`, forwards a team's source into one of the team's sinks. A team's operators manage its routes themselves, and an administrator may manage any. A route may mute reply kinds: `comment`, `review`, `pipeline`, `push`, `merge` and `close`. Muted replies are not posted, but the thread's root post still shows every change.

Operators see only their teams and what those own. Naming another team's source or sink answers `403`. What a route uses stays until the route is removed: a source, a sink or a team with routes cannot be removed, and a team cannot give up a source or sink its routes use. A connection that sources or sinks use cannot be removed either.

Each event goes to every sink that a route connects to a source whose scope holds the event's repository. A sink that several routes reach gets the event once, and its reply unless every one of those routes mutes it. Paths match exactly, and a group holds every repository below it. An event that no route reaches is acknowledged and counted (`patchcaster_deliveries_discarded_total{cause="unrouted"}`), never posted.

A change applies to the next event, without a restart. Messages already queued keep the sink they were queued for when a route changes or goes. Each sink keeps its own thread of a merge request and its own order, so a sink that cannot deliver delays no other sink.

## API tokens

Automation authenticates with a personal API token, sent as `Authorization: Bearer <token>`. A token acts with its owner's permissions as they are at each request, so a person who loses a role loses it for their tokens at once. Create one on the interface's API tokens page, or during a login with `POST /api/v1/tokens`, naming it and, optionally, its lifetime in days (`lifetime_days`, 1 to 90, 90 by default). The response holds the token, which starts with `patchcaster_` and is shown only that once: PatchCaster keeps only a hash of it. Changes made with a token name the token in the audit log, next to its owner.

Only a login lists (`GET /api/v1/tokens`) and revokes (`DELETE /api/v1/tokens/{id}`) tokens, as the API tokens page does, so a leaked token cannot create its successor. A revoked or expired token answers `401` on its next use.

To cut someone off at once, for example when they leave, an administrator calls `POST /api/v1/persons/{id}/revoke-access`. It revokes all their tokens and ends all their sessions. They can log in again unless the identity provider refuses them, so remove them there too.

## Restoring administrator access

When no administrator can log in any more, run `patchcaster admin grant --issuer <issuer> --subject <subject>` with the database URL. It names the person exactly as their ID tokens do. The grant takes effect at once, even in a running session.

## Audit log

Administrators read every change to the configuration, every setup step, every grant and every administrator's login at `GET /api/v1/audit`, newest first. Each entry names who acted, with the API token they used, when, and what changed. Secret values never appear: a changed secret shows only as `secret changed`. Nobody can edit or delete entries.

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
| `deliveries_rejected_total` | `cause`: `endpoint`, `signature`, `parse` | deliveries refused with 404/401/400 |
| `deliveries_suppressed_total` | | duplicates suppressed by webhook id |
| `deliveries_discarded_total` | `cause`: `unprocessed_kind`, `not_allowlisted`, `unlinked`, `expired`, `unrouted` | authentic deliveries dropped without a post |
| `state_applied_total` | `kind` | accepted deliveries applied to a status board |
| `notices_total` | `notice` | thread replies posted, by notice kind |
| `push_attribution_misses_total` | | pushes that matched no open merge request |
| `messages_posted_total` | | messages Mattermost accepted |
| `messages_skipped_total` | `cause`: `no_mapping`, `thread_closed`, `duplicate_open`, `sink_gone` | queued messages deliberately not posted |
| `messages_parked_total` | `cause`: `attempt_budget`, `thread_gone`, `render_failed`, `payload_undecodable` | messages given up on (see below) |
| `mattermost_attempts_total` | `outcome`: `sent`, `retryable`, `ratelimited`, `thread_gone`, `transport` | Mattermost API attempts |
| `mattermost_attempt_duration_seconds` | | histogram of attempt durations |

A rising `deliveries_discarded_total{cause="unprocessed_kind"}` usually means
a GitLab trigger is enabled that PatchCaster does not use; a missing trigger
shows up as silence instead (see [GitLab triggers](integrations/gitlab.md#triggers)).

`outbox_listener_up` is a gauge, not a count: 1 while new messages wake the
delivery worker at once, 0 while the worker only polls (see
[database connection](#database-connection)).

## Drain and shutdown

SIGTERM stops acceptance and gives in-flight work `--drain-timeout`; set
`terminationGracePeriodSeconds` above it. Readiness turns 503 first, so the
instance leaves load balancer rotation before it stops listening.

## Upgrades and migrations

Migrations are embedded in the binary. Run `patchcaster migrate` before
rollout, or let startup apply them (single-replica v1). Both paths apply only
what is pending.

Upgrading from v0.1 to v0.2 needs a new, empty database. v0.2 does not take over a v0.1 database: it refuses to start on one and says so, and threads that v0.1 started receive no further replies. Give v0.2 a new database, or drop the old one, and configure the instance again.

## Rotating the encryption key

The [encryption key](configuration.md#encryption-key) rotates without downtime:

1. Generate a new key with `openssl rand -base64 32`.
2. Restart PatchCaster with the new key in `PATCHCASTER_ENCRYPTION_KEY` and the old one in `PATCHCASTER_PREVIOUS_ENCRYPTION_KEYS`. It starts, and warns that stored secrets are still sealed with a previous key.
3. Run `patchcaster secrets reseal` with the same two settings and the database URL. It seals every stored secret again: the identity provider's client secret, the connections' credentials and their signing tokens.
4. Restart with the new key alone, and discard the old one.

Before the reseal, a start with only the new key fails. After it, a start with only the old key fails.

## Compatibility

An upgrade needs no action unless it changes this contract:

* **Configuration**: flag and environment variable names, their meaning and defaults, and the management API under `/api/v1/`.
* **Interfaces**: the `POST /webhooks/{connection}` endpoints and their signature scheme, the probe endpoints, and metric names, label names and existing label values.
* **Requirements**: the minimum PostgreSQL and GitLab versions, the GitLab
  triggers to enable, and what the Mattermost bot needs
  ([what you need](install.md#what-you-need)).
* **Artifacts**: image and archive names, platforms, and the signing
  identities that the [verification commands](install.md#verify-what-you-deploy)
  check.

Anything else may change in any release: the wording and layout of
Mattermost posts, log messages and levels, retry and timing behaviour, and the
database schema. Migrations stay automatic, but the triage query below may
need adjusting.

A release that changes the contract lists it under **Breaking** in the
[changelog](../CHANGELOG.md), with what to do, and raises the minor version
before 1.0.0 and the major version after.

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

## Database connection

Besides its connection pool, each instance keeps one connection to
PostgreSQL that listens for new messages, so the worker delivers a message as
soon as its webhook is accepted. That connection must reach PostgreSQL
directly or through a pooler in session mode. A pooler in transaction mode,
such as PgBouncer with `pool_mode = transaction`, never forwards
notifications. PatchCaster checks for this whenever the listener connects,
logs a warning and tries again later.

Without the listener nothing is lost: the worker still picks up messages
every `--poll-interval`, only later. `patchcaster_outbox_listener_up` shows
which mode an instance is in, so alert on it staying at 0.

## Scaling

v1 runs as a **single replica**; the queue uses lease-based claims
(`--claim-lease`) so more replicas remain a future option, not a schema
change.
