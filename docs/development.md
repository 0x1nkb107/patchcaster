# Developing PatchCaster

How to build PatchCaster from source, try a change on your own machine, and
run the same checks CI runs before the change goes upstream. Operators
deploying a release want the [install guide](install.md) instead; the normative
behaviour lives in the SARA graph under [`specs/`](specs/README.md).

## Prerequisites

* **Go** at the version of the `go` directive in `go.mod` (1.27.1 today). CI
  runs with `GOTOOLCHAIN=local`, so that exact toolchain is what the gates see.
* **[just](https://just.systems)** runs every task (ADR-007); CI pins 1.58.0.
* **Node.js 24** with **npm 11.10.0 or later**, only for the web interface. `web/.nvmrc` names the Node line for nvm and similar tools. The web recipes refuse an older npm, because it would ignore the seven-day release age in `web/.npmrc` (ADR-032). Go code builds and tests without Node: a binary built before the interface serves a page that says so.
* **podman** for the development database and the container image. To use
  docker instead, export `ENGINE=docker` or override it per call:
  `just engine=docker devdb`.
* **A C toolchain**, only for `just race` — the race detector needs cgo, while
  everything else builds with `CGO_ENABLED=0`.
* **`curl`, `openssl` and `python3`**, only for the
  [local demo](../demo/README.md).
* **[sara](https://crates.io/crates/sara-cli)** (`cargo install sara-cli`),
  only when you change the specs.

Nothing else is installed by hand: sqlc, golangci-lint, goreleaser and changie are pinned as Go tool modules under `tools/` and built by your own Go toolchain through `go tool -modfile`. The web interface's tools are development dependencies pinned by `web/package-lock.json`, and `just web-install` installs them. cosign only signs releases, so only the release workflow installs it.

## Layout

| Path | What lives there |
| --- | --- |
| `cmd/patchcaster` | the binary: command line, configuration, `serve` and `migrate` |
| `internal/ingest` | webhook intake: signature, dedupe, parse, freshness, routing, durable accept |
| `internal/event` | GitLab payloads parsed into the normalized event model |
| `internal/plan` | the delivery-policy table: what an accepted delivery posts |
| `internal/deliver` | outbox worker and the embedded message templates |
| `internal/mattermost` | bot-account client that posts and updates messages, and reads the bot's account and channels |
| `internal/gitlab`, `internal/platform` | GitLab API client, and the credential and channel checks of each platform |
| `internal/store` | PostgreSQL queue, thread mappings and dedupe records; migrations, queries, generated `sqlcgen` |
| `internal/metrics`, `internal/ops` | funnel counters, probe and metrics handlers |
| `internal/graceful` | HTTP serving with a drain budget, for both listeners |
| `internal/webui` | serves the embedded web interface on the management listener |
| `web/` | the web interface: a SvelteKit single-page application, and the `web` package that embeds its bundle |
| `demo/` | stub Mattermost and signed-webhook simulator |
| `docs/specs/` | requirements, architecture and ADRs (SARA) |
| `tools/` | pinned tool modules |
| `.changes/` | changelog fragments and per-release notes (changie) |

## Tasks

`just --list` shows every recipe, grouped into dev, test, check and
release. These are the common ones:

```sh
just test             # unit and store contract tests (needs a Docker API)
just fmt              # gofmt in place
just quality          # CI gate: fmt check + CGO-free build + sqlc and API checks + vet + lint, for Go and the web interface
just lint             # vet + pinned golangci-lint
just race             # full suite under the race detector (needs cgo)
just devdb            # PostgreSQL 18 on 127.0.0.1:5433 (podman)
just migrate          # apply the embedded schema to the dev database
just run              # serve against the dev database (see below)
just test-integration # store contract tests only, in their own PostgreSQL container
just test-e2e         # end-to-end tests: the real binary against its own PostgreSQL container
just sqlc             # regenerate query code after touching internal/store/{queries,migrations}
just sqlc-check       # fail if the committed query code differs from sqlc's output
just api              # regenerate the management API code after editing internal/api/openapi-v1.yaml
just api-check        # fail if the committed API code differs from ogen's output
just web-install      # install the web interface's packages from the lockfile (npm ci)
just web-build        # build the web interface into web/dist/app, which the binary embeds
just web-lint         # svelte-check, ESLint and Prettier over the web interface
just web-fmt          # apply Prettier's formatting to the web interface
just web-api          # regenerate the web interface's API client after editing internal/api/openapi-v1.yaml
just web-api-check    # fail if the committed web API client differs from orval's output
just web-audit        # fail on known vulnerabilities in the web interface's packages
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

`just run` serves against the dev database, with a fixed encryption key and the interface at `http://127.0.0.1:8081`. Connections and routes come from the database, so a new dev database needs the [first setup](operations.md#first-setup), with `just bootstrap` for its setup link, and an identity provider to log in through.

```sh
just devdb     # once
just web-install && just web-build   # once, and again after changing the interface
just run
```

Startup applies pending migrations, so `just migrate` is only needed to migrate without serving. Webhooks go to `POST http://127.0.0.1:8080/webhooks/<connection id>`; probes and metrics answer on `:9090`. Remove the database with
`podman rm -f patchcaster-pg`. `compose.yaml` describes the same database for
`podman compose` or `docker compose` users; run one or the other, since both
bind port 5433.

To work on the interface itself, run `npm run dev` in `web/` next to `just run`. Vite then serves the interface with live reloading at `http://127.0.0.1:5173` and forwards `/api` and `/auth` to the management listener at `:8081`. Logging in still ends at `:8081`, but cookies do not depend on the port, so the session reaches the development server too.

No GitLab or Mattermost at hand? The [local demo](../demo/README.md) pairs a stub Mattermost that prints every post with a simulator that sends signed GitLab webhooks. It does not work with the stored configuration yet.

## Testing

`go test ./...` (`just test`) runs the unit tests and the store contract tests in `internal/store`. The store tests start their own PostgreSQL 18 container through testcontainers-go and recreate its schema before every test (ADR-026). They never touch the dev database. They need a Docker API and fail with an explicit message when none is reachable, so that the contract can never be skipped unnoticed.

The end-to-end tests in `e2e/` build the binary and drive it as a separate process against PostgreSQL, a fake Mattermost and signed webhooks (ADR-025). They are a separate Go module, so `go test ./...` at the root skips them, and they start a PostgreSQL container of their own too.

Docker works as it is. With podman, export these two variables first, for example in your shell profile. The first points testcontainers at podman's socket. The second switches off testcontainers' cleanup container, which rootless podman cannot run, because the tests remove their containers themselves:

```sh
export DOCKER_HOST=unix:///run/user/$(id -u)/podman/podman.sock
export TESTCONTAINERS_RYUK_DISABLED=true
```

Tests use testify with an arrange-act-assert layout (ADR-006).

## Before sending a change upstream

CI runs on every push and pull request. Its four jobs map to these local commands (`-count=1` bypasses the test cache, as CI does):

```sh
just web-install     # once: the quality gate lints the web interface
just quality         # CI "quality": gofmt check, CGO-free build, sqlc and API checks, vet, golangci-lint, web lint
just web-build       # CI "quality" too: the bundle the binary embeds
just web-audit       # CI "quality" too: known vulnerabilities in the web interface's packages
just test -count=1   # CI "test": unit and store contract tests
just race -count=1   # CI "race": the same suite under the race detector
just test-e2e        # CI "e2e": the real binary end to end
```

Then, depending on what the change touches:

* **Anything a user or operator would notice** (behaviour, configuration,
  metrics, artifacts, documented procedures) and **updated dependencies**
  that ship in the binary or image: add a changelog fragment with
  `just change` and commit the new `.changes/unreleased/*.yaml` with the
  change. Write it for operators, not developers. Refactors, tests, CI and
  development tool pins need none.
* **SQL queries or migrations** (`internal/store/queries`, `internal/store/migrations`): run `just sqlc` and commit the regenerated `internal/store/sqlcgen`. Never edit the generated code by hand: CI runs `just sqlc-check` and fails when the committed code differs from what sqlc generates. Each query file holds the queries of one table, and a comment directly after a query's `-- name:` line becomes the doc comment of the generated method. v0.2's migrations keep their history in a table of their own, `patchcaster_migrations`. Until v0.2 is released they may still be rewritten, because no v0.2 database exists yet. Afterwards they only grow.
* **Management API** (`internal/api/openapi-v1.yaml`): the description is the source. Run `just api` and commit the regenerated server in `internal/api/v1gen` and the e2e client in `e2e/apiv1`. Run `just web-api` too and commit the web interface's client in `web/src/lib/api/generated`. Never edit them by hand: CI runs `just api-check` and `just web-api-check`. Within `/api/v1/`, changes stay additive; a change that would break clients needs a new version.
* **Web interface** (`web/`): run `just web-lint` and `just web-build`. CI also runs `just web-audit`. Add or update packages with npm 11.10.0 or later, so that the lockfile only takes releases older than seven days. A fix younger than that needs a reviewed `min-release-age-exclude` entry in `web/.npmrc`. Components from shadcn-svelte live in `web/src/lib/components/ui`. Add them with `npx shadcn-svelte@1.7.0 add <component>`, and leave their code as the tool wrote it, since ESLint skips that directory.
* **Mattermost client** (`internal/mattermost`): run `just test-e2e`. Its fake Mattermost rejects any request that breaks the published API of the Mattermost release pinned in `e2e/testdata/refresh-mattermost-api.sh` (ADR-027). To move to a newer release, change the two pins at the top of that script, run it and commit the regenerated description.
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
