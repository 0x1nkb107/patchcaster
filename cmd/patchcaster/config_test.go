package main

import (
	"bytes"
	"encoding/base64"
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
	"--mattermost-token=tok", "--signing-secret=" + testSigningToken, "--routing-file=/dev/null",
	"--encryption-key=" + testKey, "--interface-url=" + testInterfaceURL,
}

// Well-formed signing tokens, as GitLab shows them. They are built at run
// time, so that secret scanners do not mistake them for live secrets.
var (
	testSigningToken  = signingTokenOf('g')
	otherSigningToken = signingTokenOf('h')
)

// signingTokenOf returns a signing token whose key is 32 bytes of letter.
func signingTokenOf(letter byte) string {
	return "whsec_" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{letter}, 32))
}

// testInterfaceURL is a well-formed interface address.
const testInterfaceURL = "https://patchcaster.example"

// testKey is a well-formed instance key: 32 bytes of 'k', base64-encoded.
const testKey = "a2tra2tra2tra2tra2tra2tra2tra2tra2tra2tra2s="

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
	t.Setenv("PATCHCASTER_SIGNING_SECRETS", testSigningToken+","+otherSigningToken)

	// act
	cli := mustParse(t, "--database-url=postgres://x", "--mattermost-url=https://mm",
		"--mattermost-token=tok", "--routing-file=/dev/null", "--encryption-key="+testKey,
		"--interface-url="+testInterfaceURL)

	// assert
	assert.Equal(t, ":7777", cli.Serve.Addr)
	assert.Equal(t, []string{testSigningToken, otherSigningToken}, cli.Serve.SigningSecrets, "env slices split on commas")
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
		"--signing-secret=not-a-token", "--encryption-key="+testKey,
		"--previous-encryption-key=not-a-key",
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
	assert.Contains(t, msg, "--previous-encryption-key 1")
	assert.NotContains(t, msg, "not-a-key", "a key never appears in an error")
	assert.Contains(t, msg, "--signing-secret 1")
	assert.NotContains(t, msg, "not-a-token", "a signing token never appears in an error")
}

func TestTheEncryptionKeyIsRequired(t *testing.T) {
	// act
	_, err := parse(t, "--database-url=postgres://x", "--mattermost-url=https://mm",
		"--mattermost-token=tok", "--signing-secret="+testSigningToken, "--routing-file=/dev/null")

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing flags: --encryption-key")
}

func TestValidateAcceptsRotationSecrets(t *testing.T) {
	// act
	cli, err := parse(t, "--database-url=postgres://x", "--mattermost-url=https://mm",
		"--mattermost-token=tok", "--routing-file=/dev/null", "--encryption-key="+testKey,
		"--interface-url="+testInterfaceURL, "--signing-secret="+testSigningToken, "--signing-secret="+otherSigningToken)

	// assert
	require.NoError(t, err)
	assert.Equal(t, []string{testSigningToken, otherSigningToken}, cli.Serve.SigningSecrets)
}

func TestTheInterfaceURLIsAnOrigin(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{"an https origin", "https://patchcaster.example", "https://patchcaster.example", ""},
		{"a trailing slash", "http://127.0.0.1:8081/", "http://127.0.0.1:8081", ""},
		{"no scheme", "patchcaster.example", "", "must start with https:// or http://"},
		{"another scheme", "ftp://patchcaster.example", "", "must start with https:// or http://"},
		{"no host", "https://", "", "must name a host"},
		{"a path", "https://example.org/patchcaster", "", "must name only a scheme, a host and a port"},
		{"a query", "https://patchcaster.example?x=1", "", "must name only a scheme, a host and a port"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			u, err := parseInterfaceURL(tc.raw)

			// assert
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "--interface-url")
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, u.String())
		})
	}
}

func TestAMalformedInterfaceURLIsReportedWithTheOthers(t *testing.T) {
	// act
	args := append([]string{}, minimalFlags...)
	_, err := parse(t, append(args, "--interface-url=example.org", "--log-level=loud")...)

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--interface-url")
	assert.Contains(t, err.Error(), "--log-level")
}
