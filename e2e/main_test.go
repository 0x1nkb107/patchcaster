//go:build e2e

// Package e2e drives the real patchcaster binary end to end (ADR-025). It
// builds the binary once, then starts, signals and restarts it as a separate
// process against PostgreSQL, a fake Mattermost and signed GitLab webhooks,
// so the tests exercise what operators run.
//
// The suite needs a disposable database in PATCHCASTER_TEST_DATABASE_URL and
// recreates its public schema before every test. Run it with
// `just test-e2e`.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// binaryPath is the patchcaster binary that TestMain builds for the suite.
var binaryPath string

// databaseURL names the disposable database that the suite owns.
var databaseURL string

func TestMain(m *testing.M) {
	os.Exit(runSuite(m))
}

// runSuite builds the binary, runs the tests and removes the build again.
func runSuite(m *testing.M) int {
	databaseURL = os.Getenv("PATCHCASTER_TEST_DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "e2e: PATCHCASTER_TEST_DATABASE_URL must name a disposable database")
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

// build compiles the binary the way releases do: static, with CGO off.
func build(output string) error {
	cmd := exec.Command("go", "build", "-trimpath", "-o", output, "../cmd/patchcaster")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build patchcaster: %w", err)
	}
	return nil
}
