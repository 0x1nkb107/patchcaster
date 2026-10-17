package main

import (
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// run parses args with main's kong options and runs the selected command.
func run(t *testing.T, args ...string) error {
	t.Helper()
	var cli CLI
	k, err := kong.New(&cli, kongOptions(t.Context())...)
	require.NoError(t, err)
	kctx, err := k.Parse(args)
	require.NoError(t, err)
	return kctx.Run()
}

func TestMigrateRunsMigrateCmd(t *testing.T) {
	// act: an unparsable URL fails inside MigrateCmd.Run, before any connection
	err := run(t, "migrate", "--database-url=postgres://u:%zz@h/db")

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse database url")
}

func TestServeIsTheDefaultCommand(t *testing.T) {
	// arrange: a missing routing file fails inside ServeCmd.Run, before any
	// connection
	missing := filepath.Join(t.TempDir(), "routing.yaml")

	// act: no command name given
	err := run(t, "--database-url=postgres://x", "--mattermost-url=https://mm",
		"--mattermost-token=tok", "--signing-secret="+testSigningToken, "--routing-file="+missing,
		"--encryption-key="+testKey, "--interface-url="+testInterfaceURL, "--public-url="+testPublicURL)

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open routing file")
}

func TestBootstrapRefusesAMalformedInterfaceURL(t *testing.T) {
	// act
	_, err := parse(t, "bootstrap", "--database-url=postgres://x", "--interface-url=https://example.org/sub")

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--interface-url")
}
