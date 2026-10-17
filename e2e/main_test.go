// Package e2e drives the real patchcaster binary end to end (ADR-025). It
// builds the binary once, then starts, signals and restarts it as a separate
// process against PostgreSQL, a fake Mattermost and signed GitLab webhooks,
// so the tests exercise what operators run.
//
// The suite is its own module, so its test dependencies stay out of the
// product's go.mod and a root-level `go test ./...` never runs it. It starts
// PostgreSQL in a container through testcontainers-go, which needs a Docker
// API: Docker itself, or podman's socket named by DOCKER_HOST. Run it with
// `just test-e2e`.
package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// postgresImage is the database the suite runs against, the same major
// version that production requires. The registry is spelled out so that
// podman needs no short-name lookup.
const postgresImage = "docker.io/library/postgres:18"

// binaryPath is the patchcaster binary that TestMain builds for the suite.
var binaryPath string

// databaseURL points at the suite's own PostgreSQL container.
var databaseURL string

// databaseContainer is that container, for the tests that read the database
// the way an operator's tools do.
var databaseContainer *postgres.PostgresContainer

func TestMain(m *testing.M) {
	os.Exit(runSuite(m))
}

// runSuite starts the database, builds the binary, runs the tests and
// removes the container and the build again.
func runSuite(m *testing.M) int {
	ctx := context.Background()
	database, err := startPostgres(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(database) }()
	databaseContainer = database

	databaseURL, err = database.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: read database address: %v\n", err)
		return 1
	}

	dir, err := os.MkdirTemp("", "patchcaster-e2e-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: create build directory: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()

	binaryPath = filepath.Join(dir, "patchcaster")
	if err := build(binaryPath); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		return 1
	}
	return m.Run()
}

// startPostgres runs a disposable PostgreSQL that belongs to this suite
// alone, so the tests can recreate its schema without harming anything.
func startPostgres(ctx context.Context) (*postgres.PostgresContainer, error) {
	database, err := postgres.Run(ctx, postgresImage,
		postgres.WithDatabase("patchcaster"),
		postgres.WithUsername("patchcaster"),
		postgres.WithPassword("e2e"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}
	return database, nil
}

// build compiles the binary the way releases do: static, with CGO off. It
// builds from the product module one directory up.
func build(output string) error {
	cmd := exec.Command("go", "build", "-trimpath", "-o", output, "./cmd/patchcaster")
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build patchcaster: %w", err)
	}
	return nil
}
