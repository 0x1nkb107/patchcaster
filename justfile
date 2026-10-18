# PatchCaster development tasks (ADR-007).
# Toolchain binaries are pinned tools modules, never ambient installs. The
# one exception is cosign, which the release workflow installs (ADR-016).
#
# Every recipe carries one group for `just --list`: dev changes sources or
# the local environment, test runs a test suite, check only reports, and
# release prepares or cuts a release. A rare maintenance step belongs next
# to what it maintains, not here.

engine := env_var_or_default("ENGINE", "podman")
database_url := "postgres://patchcaster:dev@127.0.0.1:5433/patchcaster?sslmode=disable"
# A fixed instance key for the throwaway dev database only, so that restarts
# open the secrets earlier runs sealed. Never use it for a real deployment.
dev_encryption_key := "cGF0Y2hjYXN0ZXItZGV2LWRhdGFiYXNlLWtleS0zMmI="

# Format, vet and run the unit and store contract tests (the default).
[group('dev')]
all: fmt vet test

# Format Go sources in place.
[group('dev')]
fmt:
	gofmt -w cmd internal e2e

# Vet without building tests of unrelated packages.
[group('check')]
vet:
	go vet ./...

# Variadic: `just test -count=1` forwards to go test. The store contract
# tests start their own PostgreSQL container (ADR-026), so a Docker API must
# be reachable. With podman, export DOCKER_HOST to podman's socket and set
# TESTCONTAINERS_RYUK_DISABLED=true.
# Unit and store contract tests.
[group('test')]
test +args="":
	go test {{args}} ./...

# Run the suite under the race detector; needs cgo, which -race requires.
[group('test')]
race +args="":
	CGO_ENABLED=1 go test -race {{args}} ./...

# Note: just passes $ through untouched (only {{...}} is just's syntax).
# Fail on unformatted files (CI form of fmt; fmt writes, this only reports).
[group('check')]
fmt-check:
	@unformatted="$(gofmt -l cmd internal e2e)"; test -z "$unformatted" || { printf 'unformatted files:\n%s\n' "$unformatted"; exit 1; }

# Store contract tests only, against their own PostgreSQL container.
[group('test')]
test-integration:
	go test -count=1 ./internal/store/

# The suite is its own module and starts its own PostgreSQL through a Docker
# API. With podman, export DOCKER_HOST to podman's socket and set
# TESTCONTAINERS_RYUK_DISABLED=true.
# End-to-end tests: the real binary against PostgreSQL and a fake Mattermost (ADR-025).
[group('test')]
[working-directory: 'e2e']
test-e2e +args="":
	go test -count=1 {{args}} ./...

# Needs the web interface's packages: run `just web-install` once.
# CI quality gate: formatting, the CGO-free build, generated code, vet and lint.
[group('check')]
quality: fmt-check build sqlc-check api-check lint web-lint web-api-check

# -modfile resolves the tool from the pinned tools/golangci module while the
# working directory stays at the service module root.
# Vet and the pinned golangci-lint.
[group('check')]
lint: vet
	go tool -modfile tools/golangci/go.mod golangci-lint run
	cd e2e && go vet ./... && go tool -modfile ../tools/golangci/go.mod golangci-lint run

# Compile the static binary (CGO off is the build contract).
[group('check')]
build:
	CGO_ENABLED=0 go build ./...

# Regenerate sqlc query code after editing internal/store/queries or migrations (ADR-010).
[group('dev')]
sqlc:
	go tool -modfile tools/sqlc/go.mod sqlc generate

# Generated query code is never edited by hand or left stale.
# Fail when the committed query code differs from what sqlc generates.
[group('check')]
sqlc-check:
	go tool -modfile tools/sqlc/go.mod sqlc diff

# The description is the source: the server in the binary and the e2e suite's
# client both come from it (ADR-031).
# Regenerate the management API code after editing internal/api/openapi-v1.yaml.
[group('dev')]
api: (ogen "internal/api/v1gen" "e2e/apiv1")

# ogen offers no diff mode, so this generates into a scratch directory and
# compares. Generated API code is never edited by hand or left stale.
# Fail when the committed API code differs from what ogen generates.
[group('check')]
api-check:
	#!/usr/bin/env bash
	set -euo pipefail
	scratch="$(mktemp -d)"
	trap 'rm -rf "${scratch}"' EXIT
	just ogen "${scratch}/v1gen" "${scratch}/apiv1" >/dev/null
	diff -r "${scratch}/v1gen" internal/api/v1gen
	diff -r "${scratch}/apiv1" e2e/apiv1

# Generate the v1 server and the e2e client into the given directories.
[private]
ogen server client:
	go tool -modfile tools/ogen/go.mod ogen -loglevel error -clean -config internal/api/ogen-server.yaml -package v1gen -target {{server}} internal/api/openapi-v1.yaml
	go tool -modfile tools/ogen/go.mod ogen -loglevel error -clean -config e2e/ogen-client.yaml -package apiv1 -target {{client}} internal/api/openapi-v1.yaml

# The web interface (ADR-032). Its recipes run npm in web/, whose .npmrc
# resolves only releases older than seven days. npm honours that only from
# 11.10.0 on and merely warns before, so the recipes refuse an older npm.
[private]
[working-directory: 'web']
web-npm:
	#!/usr/bin/env bash
	set -euo pipefail
	version="$(npm --version)"
	if ! printf '11.10.0\n%s\n' "${version}" | sort -V -C; then
		echo "npm ${version} ignores min-release-age in web/.npmrc; use npm 11.10.0 or later" >&2
		exit 1
	fi

# Install the web interface's packages exactly as the lockfile pins them.
[group('dev')]
[working-directory: 'web']
web-install: web-npm
	npm ci

# Bundle the web interface into web/dist/app, which the binary embeds.
[group('dev')]
[working-directory: 'web']
web-build: web-npm
	npm run build

# Type-check, lint and check the formatting of the web interface (ADR-042).
[group('check')]
[working-directory: 'web']
web-lint: web-npm
	npm run check
	npm run lint

# Format the web interface in place.
[group('dev')]
[working-directory: 'web']
web-fmt: web-npm
	npm run format

# Run after editing internal/api/openapi-v1.yaml.
# Regenerate the web interface's API client.
[group('dev')]
[working-directory: 'web']
web-api: web-npm
	npm run api

# A stale client is left regenerated.
# Fail when the committed web API client differs from what orval generates.
[group('check')]
[working-directory: 'web']
web-api-check: web-npm
	#!/usr/bin/env bash
	set -euo pipefail
	scratch="$(mktemp -d)"
	trap 'rm -rf "${scratch}"' EXIT
	cp -r src/lib/api/generated "${scratch}/committed"
	npm run --silent api >/dev/null
	if ! diff -r "${scratch}/committed" src/lib/api/generated; then
		echo "the web interface's API client is stale: commit what just web-api generated" >&2
		exit 1
	fi

# Fail on known vulnerabilities in the web interface's packages.
[group('check')]
[working-directory: 'web']
web-audit: web-npm
	npm audit --audit-level=low

# Apply migrations from the embedded set against the target database.
[group('dev')]
migrate:
	go run ./cmd/patchcaster migrate --database-url "{{database_url}}"

# Run the service locally against the dev database.
[group('dev')]
run:
	go run ./cmd/patchcaster serve \
	  --database-url "{{database_url}}" \
	  --encryption-key "{{dev_encryption_key}}" \
	  --interface-url http://127.0.0.1:8081 \
	  --public-url http://127.0.0.1:8080

# Issue a setup link for the dev instance that `just run` serves.
[group('dev')]
bootstrap:
	go run ./cmd/patchcaster bootstrap \
	  --database-url "{{database_url}}" \
	  --interface-url http://127.0.0.1:8081

# Disposable PostgreSQL 18 for local work (podman or docker via $ENGINE).
[group('dev')]
devdb:
	{{engine}} run -d --name patchcaster-pg --replace \
	  -e POSTGRES_USER=patchcaster -e POSTGRES_PASSWORD=dev -e POSTGRES_DB=patchcaster \
	  -p 127.0.0.1:5433:5432 docker.io/library/postgres:18

# The pinned host toolchain compiles, then the binary and the licence files
# are staged in the layout the shared release Dockerfile reads (ADR-015,
# ADR-024) — no compiler ever runs inside the image. The binary embeds the
# web interface, which is built first.
# Build the OCI image locally (single architecture; pass `arm64` for arm64).
[group('dev')]
container arch="amd64": web-build
	CGO_ENABLED=0 GOARCH={{arch}} go build -trimpath -ldflags "-s -w" -o dist/container/linux/{{arch}}/patchcaster ./cmd/patchcaster
	mkdir -p dist/container/LICENSES
	cp LICENSE dist/container/LICENSE
	cp LICENSES/LicenseRef-PatchCaster-Accepted-AGPL.txt dist/container/LICENSES/
	{{engine}} build --pull=missing -t patchcaster:dev --platform=linux/{{arch}} -f Dockerfile --build-arg TARGETPLATFORM=linux/{{arch}} dist/container

# Prompts for the kind and body; flags skip the prompts, e.g.
# `just change -k Fixed -b "Retry posts after a Mattermost restart."`.
# Record a changelog fragment for the next release (ADR-018).
[group('release')]
[positional-arguments]
change *args:
	go tool -modfile tools/changie/go.mod changie new "$@"

# The fragments go to .changes/<version>.md, and CHANGELOG.md is rebuilt.
# changie picks the version from the fragments' kinds; pass major, minor or
# patch, or a tag such as v1.0.0, to override it.
# Batch the unreleased fragments into the next release's notes (ADR-018).
[group('release')]
release-changelog version="auto":
	#!/usr/bin/env bash
	set -euo pipefail
	{{ assert(version =~ '^(auto|major|minor|patch|v[0-9].*)$', "version must be auto, major, minor, patch or a tag such as v1.0.0") }}
	go tool -modfile tools/changie/go.mod changie batch --allow-no-changes=false "{{version}}"
	go tool -modfile tools/changie/go.mod changie merge
	version="$(go tool -modfile tools/changie/go.mod changie latest)"
	echo "batched ${version}: review and commit it, then run 'just release-tag'"

# The tag message is that release's notes, and git signs the tag when
# tag.gpgSign is set. Pushing the tag is left to you, because the push
# publishes it.
# Tag HEAD with the version that release-changelog last batched (ADR-018).
[group('release')]
release-tag:
	#!/usr/bin/env bash
	set -euo pipefail
	version="$(go tool -modfile tools/changie/go.mod changie latest)"
	notes=".changes/${version}.md"
	if ! git cat-file -e "HEAD:${notes}" 2>/dev/null; then
		echo "no committed ${notes}: run 'just release-changelog' and commit its result first" >&2
		exit 1
	fi
	git tag -a "${version}" --cleanup=whitespace -F "${notes}"
	echo "tagged ${version}: run 'git push origin ${version}' to release it"

# Validate the release pipeline config against the pinned tool (ADR-015).
[group('release')]
release-check:
	go tool -modfile tools/goreleaser/go.mod goreleaser check

# Keyless signing has no identity outside Actions (ADR-016). Extra args go
# to goreleaser, e.g. `--skip=docker` without a daemon. Its hooks install
# and build the web interface, which needs Node (ADR-032).
# Release dry run: builds binaries and images, publishes and signs nothing.
[group('release')]
release-snapshot +args="":
	go tool -modfile tools/goreleaser/go.mod goreleaser release --snapshot --clean --skip=publish,sign {{args}}

# This normally runs tag-triggered in CI, not here, because keyless signing
# needs the workflow's OIDC identity. Extra args go to goreleaser: the CI
# release rehearsal passes `--skip=sign` (ADR-023). goreleaser calls cosign
# from PATH, where the release workflow's installer puts it (ADR-016), and
# signs before it publishes anything. The release notes are the tag's
# batched changelog; an untagged HEAD or a tag without notes fails here,
# before anything is built.
# Cut and publish a release from the tagged HEAD (normally CI's job).
[group('release')]
[positional-arguments]
release-publish *args:
	#!/usr/bin/env bash
	set -euo pipefail
	tag="$(git describe --tags --exact-match)"
	notes=".changes/${tag}.md"
	if [[ ! -f "${notes}" ]]; then
		echo "no ${notes}: run 'just release-changelog ${tag}' and commit it before tagging" >&2
		exit 1
	fi
	exec go tool -modfile tools/goreleaser/go.mod goreleaser release --clean --release-notes "${notes}" "$@"
