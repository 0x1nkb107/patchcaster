# PatchCaster development tasks (ADR-007).
# Toolchain binaries are pinned tools modules, never ambient installs. The
# one exception is cosign, which the release workflow installs (ADR-016).

engine := env_var_or_default("ENGINE", "podman")
database_url := "postgres://patchcaster:dev@127.0.0.1:5433/patchcaster?sslmode=disable"

all: fmt vet test

# Format Go sources in place.
fmt:
	gofmt -w cmd internal e2e

# Vet without building tests of unrelated packages.
vet:
	go vet ./...

# Variadic: `just test -count=1` forwards to go test. The store contract
# tests start their own PostgreSQL container (ADR-026), so a Docker API must
# be reachable. With podman, export DOCKER_HOST to podman's socket and set
# TESTCONTAINERS_RYUK_DISABLED=true.
# Unit and store contract tests.
test +args="":
	go test {{args}} ./...

# Run the suite under the race detector; needs cgo, which -race requires.
race +args="":
	CGO_ENABLED=1 go test -race {{args}} ./...

# Fail on unformatted files (CI form of fmt; fmt writes, this only reports).
# Note: just passes $ through untouched (only {{...}} is just's syntax).
fmt-check:
	@unformatted="$(gofmt -l cmd internal e2e)"; test -z "$unformatted" || { printf 'unformatted files:\n%s\n' "$unformatted"; exit 1; }

# Store contract tests only, against their own PostgreSQL container.
test-integration:
	go test -count=1 ./internal/store/

# The suite is its own module and starts its own PostgreSQL through a Docker
# API. With podman, export DOCKER_HOST to podman's socket and set
# TESTCONTAINERS_RYUK_DISABLED=true.
# End-to-end tests: the real binary against PostgreSQL and a fake Mattermost (ADR-025).
[working-directory: 'e2e']
test-e2e +args="":
	go test -count=1 {{args}} ./...

# CI quality gate: formatting, the CGO-free build contract, generated query
# code that matches sqlc's output, and vet with the pinned golangci-lint.
quality: fmt-check build sqlc-check lint

# Vet and the pinned golangci-lint.
# -modfile resolves the tool from the pinned tools/golangci module while the
# working directory stays at the service module root.
lint: vet
	go tool -modfile tools/golangci/go.mod golangci-lint run
	cd e2e && go vet ./... && go tool -modfile ../tools/golangci/go.mod golangci-lint run

# Compile the static binary (CGO off is the build contract).
build:
	CGO_ENABLED=0 go build ./...

# Regenerate sqlc query code after editing internal/store/queries or migrations (ADR-010).
sqlc:
	go tool -modfile tools/sqlc/go.mod sqlc generate

# Generated query code is never edited by hand or left stale: fail when the
# committed code differs from what sqlc generates.
sqlc-check:
	go tool -modfile tools/sqlc/go.mod sqlc diff

# Apply migrations from the embedded set against the target database.
migrate:
	go run ./cmd/patchcaster migrate --database-url "{{database_url}}"

# Run the service locally against the dev database.
run:
	go run ./cmd/patchcaster serve \
	  --database-url "{{database_url}}" \
	  --routing-file ./contrib/routing.example.yaml

# Disposable PostgreSQL 18 for local work (podman or docker via $ENGINE).
devdb:
	{{engine}} run -d --name patchcaster-pg --replace \
	  -e POSTGRES_USER=patchcaster -e POSTGRES_PASSWORD=dev -e POSTGRES_DB=patchcaster \
	  -p 127.0.0.1:5433:5432 docker.io/library/postgres:18

# Build the OCI image locally (single architecture; pass `arm64` to build
# that one). The pinned host toolchain compiles, then the binary and the
# licence files are staged in the layout the shared release Dockerfile reads
# (ADR-015, ADR-024) — no compiler ever runs inside the image.
container arch="amd64":
	CGO_ENABLED=0 GOARCH={{arch}} go build -trimpath -ldflags "-s -w" -o dist/container/linux/{{arch}}/patchcaster ./cmd/patchcaster
	mkdir -p dist/container/LICENSES
	cp LICENSE dist/container/LICENSE
	cp LICENSES/LicenseRef-PatchCaster-Accepted-AGPL.txt dist/container/LICENSES/
	{{engine}} build --pull=missing -t patchcaster:dev --platform=linux/{{arch}} -f Dockerfile --build-arg TARGETPLATFORM=linux/{{arch}} dist/container

# Record a changelog fragment for the next release (ADR-018). Prompts for the
# kind and body; flags skip the prompts, e.g.
# `just change -k Fixed -b "Retry posts after a Mattermost restart."`.
[positional-arguments]
change *args:
	go tool -modfile tools/changie/go.mod changie new "$@"

# Batch the unreleased fragments into .changes/<version>.md and rebuild
# CHANGELOG.md (ADR-018). changie picks the version from the fragments' kinds;
# pass major, minor or patch, or a tag such as v1.0.0, to override it.
release-changelog version="auto":
	#!/usr/bin/env bash
	set -euo pipefail
	{{ assert(version =~ '^(auto|major|minor|patch|v[0-9].*)$', "version must be auto, major, minor, patch or a tag such as v1.0.0") }}
	go tool -modfile tools/changie/go.mod changie batch --allow-no-changes=false "{{version}}"
	go tool -modfile tools/changie/go.mod changie merge
	version="$(go tool -modfile tools/changie/go.mod changie latest)"
	echo "batched ${version}: review and commit it, then run 'just release-tag'"

# Tag HEAD with the version that `just release-changelog` last batched
# (ADR-018). The tag message is that release's notes, and git signs the tag
# when tag.gpgSign is set. Pushing the tag is left to you, because the push
# publishes it.
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
release-check:
	go tool -modfile tools/goreleaser/go.mod goreleaser check

# Full release dry run: builds binaries and images, publishes and signs
# nothing (keyless signing has no identity outside Actions, ADR-016).
# Extra args go to goreleaser, e.g. `--skip=docker` without a daemon.
release-snapshot +args="":
	go tool -modfile tools/goreleaser/go.mod goreleaser release --snapshot --clean --skip=publish,sign {{args}}

# Cut and publish a release; normally runs tag-triggered in CI, not here
# (keyless signing needs the workflow's OIDC identity). Extra args go to
# goreleaser: the CI release rehearsal passes `--skip=sign` (ADR-023).
# goreleaser calls cosign from PATH, where the release workflow's installer
# puts it (ADR-016), and signs before it publishes anything. The release
# notes are the tag's batched changelog; an untagged HEAD or a tag without
# notes fails here, before anything is built.
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
