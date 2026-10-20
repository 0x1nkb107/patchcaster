# Operations

Running PatchCaster once it is [installed](install.md): what to watch, how it
shuts down and upgrades, and what its delivery guarantees mean in practice.

## Web interface and management API

The management listener (`--management-addr`, `:8081` by default) serves the web interface at the interface URL, the management API under `/api/v1/`, and the API's OpenAPI description at `GET /api/v1/openapi.yaml`. It answers nothing else, and the webhook and internal listeners never answer interface or API paths, so each listener can be exposed differently: the webhook listener to the forges, the management listener only to the organization's people and automation. Errors are RFC 9457 problem details (`application/problem+json`). A validation error lists each field at fault by its path, such as `allowed_groups[0]`. Lists page with `limit` and the `next_cursor` of the previous page. Every change sends the `ETag` of what it read in `If-Match`, and fails with `412 Precondition Failed` when the resource changed meanwhile.

The web interface covers the first setup, the login and its settings, the connections with their signing tokens, the teams with their sources and sinks, API tokens and the [health](#connection-and-sink-health) of connections and sinks so far. Everything else goes through the API, which the interface uses too. The interface runs only its own scripts, and no other site may show it in a frame. Every page links to the licences of the third-party packages it contains.

The management listener also serves the login. People log in through the organization's OpenID Connect provider, and PatchCaster keeps no passwords. A session ends after one hour without activity, twelve hours after login at the latest, or at logout (`DELETE /api/v1/session`). The API refuses any change that a browser sends from another site.

## First setup

A new instance has no identity provider and no Instance Admin yet:

1. Register PatchCaster as a confidential client at the identity provider, with the authorization code flow and the redirect address `<interface URL>/auth/callback`.
2. Run `patchcaster bootstrap`. It prints a setup link of the form `<interface URL>/setup#<token>`, which works once, for one hour.
3. Open the setup link in the browser that will log in, and enter the provider's issuer, the client ID and the client secret. PatchCaster checks that a provider answers at the issuer, seals the client secret, and sends the browser on to the login. Without the interface, send the token from the link and the same values to `POST /api/v1/setup` from that browser. It answers with the login address.
4. Log in in the same browser. You become the first Instance Admin, and the setup link stops working.

Everyone who logs in later starts as a Regular User. To admit only some of the people the provider knows, name the ID token claim that lists a person's groups (`groups_claim`) and the groups that may log in (`allowed_groups`), at setup or later on the interface's Login page or through `PUT /api/v1/settings/login`. A refused login tells the person why.

## Roles

PatchCaster knows three roles, and each may also do what the roles below it may:

- An **Instance Admin** sets up the instance and decides who may do what: the login, the connections, the teams with their sources and sinks, and the roles. An Instance Admin makes another person an Instance Admin with `PATCH /api/v1/persons/{id}`.
- A **Routing Manager** routes the sources of their teams into the teams' sinks. An Instance Admin makes a person a Routing Manager by adding them to a team's `managers`.
- A **Regular User** manages their own API tokens.

## Connections

A connection holds what PatchCaster acts with on one platform: GitLab as a forge, Mattermost as a chat platform. Instance Admins manage them on the interface's Connections page, or at `/api/v1/connections`. A connection's credential is checked against its platform before it is stored, and the connection shows the account the credential acts as. A token the platform refuses is not stored, and the answer names the platform's reason. The credential is stored encrypted and is never shown again: send a new one to replace it.

- A **Mattermost** connection needs the bot account's token.
- A **GitLab** connection with an access token gets its hooks from PatchCaster: see [registered hooks](#registered-hooks). Without a token, register the hooks by hand at the connection's `webhook_url`. Then generate a signing token in each hook's form and add it to the connection under `/api/v1/connections/{id}/signing-tokens`, exactly as GitLab shows it. A connection holds several signing tokens at once, so a token rotates without downtime: add the new one, switch the hook over, then remove the old one.

A connection that sources or sinks use cannot be removed, and neither can one whose registered hooks are still being removed. A changed credential or signing token applies to the next delivery, without a restart.

### Registered hooks

With an access token, PatchCaster registers the hooks a GitLab connection's sources need, in the background, right after a source or the connection changes. It registers one hook for every scope that no broader source covers: a project hook for a repository, a group hook for a group, and a system hook for the whole instance. A broader source replaces the narrower hooks it covers, and removing it brings them back. Each hook gets a signing token that PatchCaster generates, adds to the connection and removes again with the hook.

- GitLab's system hooks deliver merge request and push events only. With an instance source, the narrower sources keep hooks for comments and pipelines. Repositories that only the instance source reaches get no comments or pipelines.
- The token needs the Maintainer role in each repository and the Owner role in each group. For the instance, it must belong to a GitLab administrator. Group hooks also need GitLab Premium or Ultimate.
- A hook GitLab refuses is tried again every five minutes. Each source shows in `hook` how its events arrive: `registered`, `pending`, `failed` with GitLab's answer, `covered` with the broader scope, or `manual` without a token.
- PatchCaster names its hooks "PatchCaster". Hooks registered by hand stay as they are, so do not register one by hand where PatchCaster registers one too: every event would arrive twice.

## Teams and routes

Teams decide who may route what. Instance Admins create teams on the interface's Teams page or at `/api/v1/teams`, name each team's Routing Managers, and give it sources and sinks. A source or sink may belong to several teams, such as a shared release channel.

- A **source** selects the repositories of a forge connection, on the Sources page or at `/api/v1/sources`: the whole instance, a group with everything below it, or one repository.
- A **sink** names a channel of a chat connection, on the Sinks page or at `/api/v1/sinks`. PatchCaster checks that the connection's bot can post there before it stores the sink, and names the reason when it cannot, such as a bot that is not a member of the channel.
- A **route**, at `/api/v1/routes`, forwards a team's source into one of the team's sinks. A team's Routing Managers manage its routes themselves, and an Instance Admin may manage any. A route may mute reply kinds: `comment`, `review`, `pipeline`, `push`, `merge` and `close`. Muted replies are not posted, but the thread's root post still shows every change. A route may also replace the built-in [message templates](templates.md#templates-of-a-route) with its own.

Routing Managers see only their teams and what those own. Naming another team's source or sink answers `403`. What a route uses stays until the route is removed: a source, a sink or a team with routes cannot be removed, and a team cannot give up a source or sink its routes use. A connection that sources or sinks use cannot be removed either.

Each event goes to every sink that a route connects to a source whose scope holds the event's repository. A sink that several routes reach gets the event once, and its reply unless every one of those routes mutes it. Paths match exactly, and a group holds every repository below it. An event that no route reaches is acknowledged and counted (`patchcaster_deliveries_discarded_total{cause="unrouted"}`), never posted.

A change applies to the next event, without a restart. Messages already queued keep the sink they were queued for when a route changes or goes. Each sink keeps its own thread of a merge request and its own order, so a sink that cannot deliver delays no other sink.

## API tokens

Automation authenticates with a personal API token, sent as `Authorization: Bearer <token>`. A token acts with its owner's permissions as they are at each request, so a person who loses a role loses it for their tokens at once. Create one on the interface's API tokens page, or during a login with `POST /api/v1/tokens`, naming it and, optionally, its lifetime in days (`lifetime_days`, 1 to 90, 90 by default). The response holds the token, which starts with `patchcaster_` and is shown only that once: PatchCaster keeps only a hash of it. Changes made with a token name the token in the audit log, next to its owner.

Only a login lists (`GET /api/v1/tokens`) and revokes (`DELETE /api/v1/tokens/{id}`) tokens, as the API tokens page does, so a leaked token cannot create its successor. A revoked or expired token answers `401` on its next use.

To cut someone off at once, for example when they leave, an Instance Admin calls `POST /api/v1/persons/{id}/revoke-access`. It revokes all their tokens and ends all their sessions. They can log in again unless the identity provider refuses them, so remove them there too.

## Restoring Instance Admin access

When no Instance Admin can log in any more, run `patchcaster admin grant --issuer <issuer> --subject <subject>` with the database URL. It names the person exactly as their ID tokens do. The grant takes effect at once, even in a running session.

## Audit log

Instance Admins read every change to the configuration, every setup step, every grant and every Instance Admin's login at `GET /api/v1/audit`, newest first. Each entry names who acted, with the API token they used, when, and what changed. Secret values never appear: a changed secret shows only as `secret changed`. Nobody can edit or delete entries.

## Probes and metrics

The internal listener (`--internal-addr`, `:9090` by default) serves:

| Endpoint | Answers |
| --- | --- |
| `GET /livez` | liveness; never depends on Mattermost |
| `GET /readyz` | readiness; store-backed, flips to 503 at SIGTERM |
| `GET /metrics` | Prometheus metrics, all prefixed `patchcaster_` |

The metrics follow a delivery from ingest to post. They name connections and sinks by their ids in the labels `connection` and `sink`, as the management API shows them. A delivery to an endpoint that names no connection counts with an empty `connection`.

| Metric | Labels | Counts |
| --- | --- | --- |
| `deliveries_received_total` | `connection` | webhook deliveries received |
| `deliveries_rejected_total` | `cause`: `endpoint`, `signature`, `parse`; `connection` | deliveries refused with 404/401/400 |
| `deliveries_suppressed_total` | `connection` | duplicates suppressed by webhook id |
| `deliveries_discarded_total` | `cause`: `unprocessed_kind`, `not_allowlisted`, `unlinked`, `expired`, `unrouted`; `connection` | authentic deliveries dropped without a post |
| `state_applied_total` | `kind`, `connection` | accepted deliveries applied to a status board |
| `notices_total` | `notice` | thread replies posted, by notice kind |
| `push_attribution_misses_total` | `connection` | pushes that matched no open merge request |
| `messages_posted_total` | `connection`, `sink` | messages Mattermost accepted |
| `messages_skipped_total` | `cause`: `no_mapping`, `thread_closed`, `duplicate_open`, `sink_gone`; `connection`, `sink` | queued messages deliberately not posted |
| `messages_parked_total` | `cause`: `attempt_budget`, `thread_gone`, `render_failed`, `payload_undecodable`; `connection`, `sink` | messages given up on (see below) |
| `template_fallbacks_total` | `kind`, `connection`, `sink` | messages written with the built-in template because the route's own failed |
| `mattermost_attempts_total` | `outcome`: `sent`, `retryable`, `ratelimited`, `thread_gone`, `transport`; `connection`, `sink` | Mattermost API attempts |
| `mattermost_attempt_duration_seconds` | `connection` | histogram of attempt durations |

Two gauges show what waits for each sink. Every replica reads them from the shared queue, so all replicas report the same values:

| Metric | Labels | Shows |
| --- | --- | --- |
| `sink_waiting_messages` | `connection`, `sink` | messages queued for the sink and not yet delivered |
| `sink_oldest_waiting_seconds` | `connection`, `sink` | the age of the oldest of them, 0 when none waits |

A sink whose oldest message keeps ageing cannot deliver. The [health page](#connection-and-sink-health) says why.

A rising `deliveries_discarded_total{cause="unprocessed_kind"}` usually means
a GitLab trigger is enabled that PatchCaster does not use; a missing trigger
shows up as silence instead (see [GitLab triggers](integrations/gitlab.md#triggers)).

`outbox_listener_up` is a gauge, not a count: 1 while new messages wake the
delivery worker at once, 0 while the worker only polls (see
[database connection](#database-connection)).

## Connection and sink health

The interface's health page shows how each connection and sink works, and so do their resources in the management API, under `health`. Instance Admins see every connection and sink, Routing Managers the sinks of their teams.

- A **sink** shows its waiting messages and the oldest of them, when Mattermost last accepted a message for it, and when an attempt last failed, with Mattermost's answer or the network error. It is failing while its last attempt failed.
- A **chat connection** sums up its sinks.
- A **forge connection** shows when its last authentic delivery arrived, and when and why a delivery was last rejected. A rejected signature usually means that a hook signs with a token the connection does not hold.

A connection or sink in trouble never changes readiness, so the instance stays in rotation and every other sink keeps delivering.

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
message parks. Each sink retries on its own, and its [health](#connection-and-sink-health) shows the last failure with Mattermost's answer. Parked messages (`patchcaster_messages_parked_total`) surface
store reasons and stay in the `outbox` table for the infrastructure team to triage:

```sql
SELECT id, mr_ref, sink_id, dest_channel, attempts, settled_reason, last_error, settled_at
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
