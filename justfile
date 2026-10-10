# PatchCaster development tasks (ADR-005).
# Toolchain binaries are pinned tools modules, never ambient installs.

engine := "podman"
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

# Build the OCI image.
container:
	{{engine}} build -t patchcaster:dev .

# Validate the release pipeline config against the pinned tool (ADR-015).
release-check:
	go tool -modfile tools/goreleaser/go.mod goreleaser check

# Full release dry run: builds binaries and images, publishes nothing.
release-snapshot:
	go tool -modfile tools/goreleaser/go.mod goreleaser release --snapshot --clean

# Cut and publish a release; normally runs tag-triggered in CI, not here.
release:
	go tool -modfile tools/goreleaser/go.mod goreleaser release --clean
