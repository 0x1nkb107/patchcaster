package store_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// postgresImage is the database the store contract runs against, the major
// version that production requires (ADR-010). The registry is spelled out
// so that podman needs no short-name lookup.
const postgresImage = "docker.io/library/postgres:18"

// databaseURL points at the PostgreSQL container that TestMain starts for
// this package's tests alone.
var databaseURL string

// TestMain starts a disposable PostgreSQL container for the store contract
// tests (ADR-026). The tests fail when no Docker API is reachable rather
// than skip, so that a missing database can never hide the contract.
func TestMain(m *testing.M) {
	os.Exit(runWithDatabase(m))
}

func runWithDatabase(m *testing.M) int {
	ctx := context.Background()
	database, err := postgres.Run(ctx, postgresImage,
		postgres.WithDatabase("patchcaster"),
		postgres.WithUsername("patchcaster"),
		postgres.WithPassword("store-tests"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "store tests: start postgres container (they need a Docker API: "+
			"Docker, or podman's socket in DOCKER_HOST): %v\n", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(database) }()

	databaseURL, err = database.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "store tests: read database address: %v\n", err)
		return 1
	}
	return m.Run()
}
