# Developing PatchCaster

How to build PatchCaster from source, try a change on your own machine, and
run the same checks CI runs before the change goes upstream. Operators
deploying a release want the [install guide](install.md) instead; the normative
behaviour lives in the SARA graph under [`specs/`](specs/README.md).

## Prerequisites

* **Go** at the version of the `go` directive in `go.mod` (1.27.1 today). CI
  runs with `GOTOOLCHAIN=local`, so that exact toolchain is what the gates see.
* **[just](https://just.systems)** runs every task (ADR-007); CI pins 1.58.0.
* **podman** for the development database and the container image. To use
  docker instead, export `ENGINE=docker` or override it per call:
  `just engine=docker devdb`.
* **A C toolchain**, only for `just race` — the race detector needs cgo, while
  everything else builds with `CGO_ENABLED=0`.
* **`curl`, `openssl` and `python3`**, only for the
  [local demo](../demo/README.md).
* **[sara](https://crates.io/crates/sara-cli)** (`cargo install sara-cli`),
  only when you change the specs.

Nothing else is installed by hand: sqlc, golangci-lint, goreleaser and changie
are pinned as Go tool modules under `tools/` and built by your own Go
toolchain through `go tool -modfile`. cosign only signs releases, so only
the release workflow installs it.

## Layout

| Path | What lives there |
| --- | --- |
| `cmd/patchcaster` | the binary: command line, configuration, `serve` and `migrate` |
| `internal/ingest` | webhook intake: signature, dedupe, parse, freshness, routing, durable accept |
| `internal/event` | GitLab payloads parsed into the normalized event model |
| `internal/plan` | the delivery-policy table: what an accepted delivery posts |
| `internal/deliver` | outbox worker and the embedded message templates |
| `internal/mattermost` | bot-account client that posts and updates messages |
| `internal/store` | PostgreSQL queue, thread mappings and dedupe records; migrations, queries, generated `sqlcgen` |
| `internal/routing`, `internal/metrics`, `internal/ops` | routing table, funnel counters, probe and metrics handlers |
| `internal/graceful` | HTTP serving with a drain budget, for both listeners |
| `contrib/` | the example routing table `just run` serves with |
| `demo/` | stub Mattermost and signed-webhook simulator |
| `docs/specs/` | requirements, architecture and ADRs (SARA) |
| `tools/` | pinned tool modules |
| `.changes/` | changelog fragments and per-release notes (changie) |

## Tasks

```sh
just test             # offline unit suite (store contract tests skip)
just fmt              # gofmt in place
just quality          # CI gate: fmt check + CGO-free build + vet + lint
just lint             # vet + pinned golangci-lint
just race             # full suite under the race detector (needs cgo)
just devdb            # PostgreSQL 18 on 127.0.0.1:5433 (podman)
just migrate          # apply the embedded schema to the dev database
just run              # serve with the example routing table (see below)
just test-integration # store contract tests against the dev database
just sqlc             # regenerate query code after touching internal/store/{queries,migrations}
just container        # OCI image patchcaster:dev (`just container arm64` for arm64)
just change           # record a changelog fragment (prompts for kind and body)
just release-changelog # batch the fragments into CHANGELOG.md as the next version
just release-tag      # tag HEAD with that version (pushing the tag releases)
just release-check    # validate the goreleaser pipeline (ADR-015)
just release-snapshot # release dry run: builds; publishes and signs nothing
```

`just test` and `just race` forward extra flags to `go test` over all
packages, e.g. `just test -run TestRenderer -v`.

## Run it locally

`just run` serves against the dev database with
[`contrib/routing.example.yaml`](../contrib/routing.example.yaml). The
remaining required settings come from the environment; the
[configuration reference](configuration.md) lists all of them.

```sh
just devdb     # once
PATCHCASTER_MATTERMOST_URL=https://mattermost.example.com \
PATCHCASTER_MATTERMOST_TOKEN=<bot account token> \
PATCHCASTER_SIGNING_SECRETS=<webhook signing token> \
just run
```

Startup applies pending migrations, so `just migrate` is only needed to
migrate without serving. Webhooks go to `POST http://127.0.0.1:8080/webhook`;
probes and metrics answer on `:9090`. Remove the database with
`podman rm -f patchcaster-pg`. `compose.yaml` describes the same database for
`podman compose` or `docker compose` users; run one or the other, since both
bind port 5433.

No GitLab or Mattermost at hand? The [local demo](../demo/README.md) pairs a
stub Mattermost that prints every post with a simulator that sends signed
GitLab webhooks, so you can watch the live board change event by event.

## Testing

`go test ./...` (`just test`) is fully offline. The store contract tests in
`internal/store` need a real PostgreSQL and skip unless
`PATCHCASTER_TEST_DATABASE_URL` points at one. They **drop and recreate its
`public` schema**, so only ever point them at a disposable database (CI
provides one).

`just test-integration` runs them against the dev database from
`just devdb`, which wipes whatever `just run` stored there. Set
`PATCHCASTER_TEST_DATABASE_URL` to another database to keep it.

Tests use testify with an arrange-act-assert layout (ADR-006).

## Before sending a change upstream

CI runs on every push and pull request. Its three jobs map to three local
commands; run them against the dev database so the store contract tests take
part (`-count=1` bypasses the test cache, as CI does):

```sh
just devdb
export PATCHCASTER_TEST_DATABASE_URL='postgres://patchcaster:dev@127.0.0.1:5433/patchcaster?sslmode=disable'
just quality         # CI "quality": gofmt check, CGO-free build, vet, golangci-lint
just test -count=1   # CI "test": unit and store contract tests
just race -count=1   # CI "race": the same suite under the race detector
```

Then, depending on what the change touches:

* **Anything a user or operator would notice** (behaviour, configuration,
  metrics, artifacts, documented procedures) and **updated dependencies**
  that ship in the binary or image: add a changelog fragment with
  `just change` and commit the new `.changes/unreleased/*.yaml` with the
  change. Write it for operators, not developers. Refactors, tests, CI and
  development tool pins need none.
* **SQL queries or migrations** (`internal/store/queries`,
  `internal/store/migrations`): run `just sqlc` and commit the regenerated
  `internal/store/sqlcgen`. CI does not regenerate it.
* **Release pipeline** (`.goreleaser.yaml`, `Dockerfile`, build flags): run `just release-check` and `just release-snapshot`. Without a Docker daemon, `just release-snapshot --skip=docker` still builds the binaries. Pull requests run `just release-check` in CI. Every push to `main` goes further in the Release workflow's rehearsal job: it batches the changelog, tags and runs `just release-publish` inside the runner, pushes the images to a throwaway registry and checks the result, without publishing anything (ADR-023).
* **Behaviour**: the specs are normative, so a change in what PatchCaster does
  needs the matching requirement, architecture item or ADR under
  [`specs/`](specs/README.md) updated too. Run `sara check` after editing
  them; CI does not.

Commit subjects carry a type prefix as in the existing history (`feat`,
`fix`, `refactor`, `build`, `docs`, `chore`). The body explains why, and
names the SYSREQ and ADR ids the change implements or amends.

## Changelog and releases

[`CHANGELOG.md`](../CHANGELOG.md) follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and is generated by
[changie](https://changie.dev) (ADR-018); never edit it by hand. Each change
carries a fragment in `.changes/unreleased/` of one kind:

| Kind | For | Bump |
| --- | --- | --- |
| Breaking | changes to the [compatibility contract](operations.md#compatibility); say what to do | minor (major from 1.0.0) |
| Added | new features | minor |
| Changed | other changes to existing user-facing behaviour; no action needed | minor |
| Deprecated | features that a later release will remove | minor |
| Removed | features that are gone | minor |
| Fixed | bug fixes | patch |
| Security | fixed vulnerabilities | patch |
| Updated | updated dependencies: Go modules, the Go toolchain, the base image | patch |

A change that needs action on upgrade takes Breaking, even when it also
removes or changes something. Flags skip the prompts:

```sh
just change -k Fixed -b "Retry posts after a Mattermost restart."
```

To cut a release:

1. `just release-changelog` batches the fragments into
   `.changes/<version>.md` and rebuilds `CHANGELOG.md`. changie picks the
   version from the kinds (the Bump column above); an argument such as
   `minor` or `v1.0.0` overrides it. It refuses an empty batch.
2. Review both files, then commit them together with the removed fragments.
3. `just release-tag` tags that commit with the new version, using the
   release notes as the tag message.
4. `git push origin <version>` publishes it. The Release workflow uses
   `.changes/<version>.md` as the GitHub release notes, and fails before
   building anything if that file is missing.
5. Tag the [charts repository](https://github.com/patchcaster-app/charts)
   with the same version so the chart's `appVersion` follows (ADR-017).

Breaking bumps the minor version while the project is on 0.x. Cut 1.0.0 with
`just release-changelog major`, then set Breaking's `auto` level in
`.changie.yaml` to `major`.
