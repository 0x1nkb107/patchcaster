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
test:
	go test ./...

# Store contract tests against a real PostgreSQL (disposable database!).
test-integration:
	PATCHCASTER_TEST_DATABASE_URL="{{test_database_url}}" go test -count=1 ./internal/store/

# Full quality gate: vet and the pinned golangci-lint (plus gofmt in CI).
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
