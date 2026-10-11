package main

import (
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parse(t *testing.T, args ...string) (*CLI, error) {
	t.Helper()
	var cli CLI
	k, err := kong.New(&cli, kong.Vars{"version": "test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = k.Parse(args)
	return &cli, err
}

// minimalFlags avoids kong's required-option noise when a test targets one
// specific rule.
var minimalFlags = []string{
	"--database-url=postgres://x", "--mattermost-url=https://mm",
	"--mattermost-token=tok", "--signing-secret=s", "--routing-file=/dev/null",
}

func mustParse(t *testing.T, args ...string) *CLI {
	t.Helper()
	cli, err := parse(t, args...)
	require.NoError(t, err)
	return cli
}

func TestDefaults(t *testing.T) {
	// act
	cli := mustParse(t, minimalFlags...)

	// assert
	assert.Equal(t, ":8080", cli.Serve.Addr)
	assert.Equal(t, ":9090", cli.Serve.InternalAddr)
	assert.Equal(t, 30*time.Second, cli.Serve.DrainTimeout)
	assert.Equal(t, 4*time.Hour, cli.Serve.EventMaxAge)
	assert.Equal(t, 24*time.Hour, cli.Serve.SeenRetention)
	assert.Equal(t, 10*time.Second, cli.Serve.PollInterval)
	assert.Equal(t, "info", cli.Serve.LogLevel)
}

func TestEnvFillsUnsetFlags(t *testing.T) {
	// arrange
	t.Setenv("PATCHCASTER_ADDR", ":7777")
	t.Setenv("PATCHCASTER_SIGNING_SECRETS", "s1,s2")

	// act
	cli := mustParse(t, "--database-url=postgres://x", "--mattermost-url=https://mm",
		"--mattermost-token=tok", "--routing-file=/dev/null")

	// assert
	assert.Equal(t, ":7777", cli.Serve.Addr)
	assert.Equal(t, []string{"s1", "s2"}, cli.Serve.SigningSecrets, "env slices split on commas")
}

func TestFlagBeatsEnv(t *testing.T) {
	// arrange: precedence is flags > environment > defaults.
	t.Setenv("PATCHCASTER_ADDR", ":7777")

	// act
	cli := mustParse(t, append([]string{"--addr=:6666"}, minimalFlags...)...)

	// assert
	assert.Equal(t, ":6666", cli.Serve.Addr)
}

func TestRequiredMissingFails(t *testing.T) {
	// act
	_, err := parse(t, "serve")

	// assert
	require.Error(t, err)
}

func TestMigrateCommandTakesURL(t *testing.T) {
	// act
	cli := mustParse(t, "migrate", "--database-url=postgres://y")

	// assert
	assert.Equal(t, "postgres://y", cli.Migrate.DatabaseURL)
}

func TestValidateInvariants(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"retention not beyond cap", append(minimalFlags, "--event-max-age=24h", "--seen-retention=24h")},
		{"retention below cap", append(minimalFlags, "--event-max-age=48h", "--seen-retention=24h")},
		{"zero drain", append(minimalFlags, "--drain-timeout=0s")},
		{"zero poll interval", append(minimalFlags, "--poll-interval=0s")},
		{"bad log level", append(minimalFlags, "--log-level=trace")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			_, err := parse(t, append([]string{"serve"}, tc.args...)...)

			// assert
			require.Error(t, err)
		})
	}
}

func TestValidateReportsAllViolationsAtOnce(t *testing.T) {
	// arrange: three independent violations plus the cross-field rule.

	// act
	_, err := parse(t, "serve",
		"--database-url=postgres://x", "--mattermost-url=https://mm",
		"--mattermost-token=tok", "--routing-file=/dev/null",
		"--signing-secret=s",
		"--log-level=trace", "--drain-timeout=0s",
		"--event-max-age=48h", "--seen-retention=24h")

	// assert: every offending flag named in one message (ADR-012),
	// not just the first.
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "--log-level")
	assert.Contains(t, msg, "--drain-timeout")
	assert.Contains(t, msg, "--seen-retention")
	assert.Contains(t, msg, "48h", "cross-field message carries the concrete cap")
}

func TestValidateAcceptsRotationSecrets(t *testing.T) {
	// act
	cli, err := parse(t, "--database-url=postgres://x", "--mattermost-url=https://mm",
		"--mattermost-token=tok", "--routing-file=/dev/null",
		"--signing-secret=a", "--signing-secret=b")

	// assert
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, cli.Serve.SigningSecrets)
}
