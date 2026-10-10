# PatchCaster development tasks (ADR-007).
# Toolchain binaries are pinned tools modules, never ambient installs. The
# one exception is cosign, which the release workflow installs (ADR-016).

engine := env_var_or_default("ENGINE", "podman")
database_url := "postgres://patchcaster:dev@127.0.0.1:5433/patchcaster?sslmode=disable"
test_database_url := env_var_or_default("PATCHCASTER_TEST_DATABASE_URL", database_url)

all: fmt vet test

# Format Go sources in place.
fmt:
	gofmt -w cmd internal

# Vet without building tests of unrelated packages.
vet:
	go vet ./...

# Fast offline unit suite (PostgreSQL tests skip without the URL).
# Variadic: `just test -count=1` forwards to go test.
test +args="":
	go test {{args}} ./...

# Run the suite under the race detector; needs cgo, which -race requires.
race +args="":
	CGO_ENABLED=1 go test -race {{args}} ./...

# Fail on unformatted files (CI form of fmt; fmt writes, this only reports).
# Note: just passes $ through untouched (only {{...}} is just's syntax).
fmt-check:
	@unformatted="$(gofmt -l cmd internal)"; test -z "$unformatted" || { printf 'unformatted files:\n%s\n' "$unformatted"; exit 1; }

# Store contract tests against a real PostgreSQL (disposable database!).
test-integration:
	PATCHCASTER_TEST_DATABASE_URL="{{test_database_url}}" go test -count=1 ./internal/store/

# CI quality gate: formatting, the CGO-free build contract, vet, and the
# pinned golangci-lint (lint pulls in vet).
quality: fmt-check build lint

# Vet and the pinned golangci-lint.
# -modfile resolves the tool from the pinned tools/golangci module while the
# working directory stays at the service module root.
lint: vet
	go tool -modfile tools/golangci/go.mod golangci-lint run

# Compile the static binary (CGO off is the build contract).
build:
	CGO_ENABLED=0 go build ./...

# Regenerate sqlc query code after editing internal/store/queries or migrations (ADR-010).
sqlc:
	go tool -modfile tools/sqlc/go.mod sqlc generate

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
# that one). The pinned host toolchain compiles, then the binary is staged
# in the layout the shared release Dockerfile reads (ADR-015) — no compiler
# ever runs inside the image.
container arch="amd64":
	CGO_ENABLED=0 GOARCH={{arch}} go build -trimpath -ldflags "-s -w" -o dist/container/linux/{{arch}}/patchcaster ./cmd/patchcaster
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
changelog version="auto":
	#!/usr/bin/env bash
	set -euo pipefail
	{{ assert(version =~ '^(auto|major|minor|patch|v[0-9].*)$', "version must be auto, major, minor, patch or a tag such as v1.0.0") }}
	go tool -modfile tools/changie/go.mod changie batch --allow-no-changes=false "{{version}}"
	go tool -modfile tools/changie/go.mod changie merge
	echo "batched $(go tool -modfile tools/changie/go.mod changie latest): review and commit it, then run 'just tag'"

# Tag HEAD with the version the last `just changelog` batched (ADR-018). The
# tag message is that release's notes, and git signs the tag when tag.gpgSign
# is set. Pushing the tag is left to you, because the push publishes it.
tag:
	#!/usr/bin/env bash
	set -euo pipefail
	version="$(go tool -modfile tools/changie/go.mod changie latest)"
	notes=".changes/${version}.md"
	if ! git cat-file -e "HEAD:${notes}" 2>/dev/null; then
		echo "no committed ${notes}: run 'just changelog' and commit its result first" >&2
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
# (keyless signing needs the workflow's OIDC identity). goreleaser calls
# cosign from PATH, where the release workflow's installer puts it
# (ADR-016), and signs before it publishes anything. The release notes are
# the tag's batched changelog; an untagged HEAD or a tag without notes fails
# here, before anything is built.
release:
	#!/usr/bin/env bash
	set -euo pipefail
	tag="$(git describe --tags --exact-match)"
	notes=".changes/${tag}.md"
	if [[ ! -f "${notes}" ]]; then
		echo "no ${notes}: run 'just changelog ${tag}' and commit it before tagging" >&2
		exit 1
	fi
	exec go tool -modfile tools/goreleaser/go.mod goreleaser release --clean --release-notes "${notes}"
