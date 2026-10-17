package e2e

import (
	"encoding/base64"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Instance keys: 32 bytes of one letter each, base64-encoded. The suite key
// is what every other test starts with.
var (
	suiteKey   = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("e", 32)))
	rotatedKey = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("r", 32)))
)

// runPatchcaster runs a patchcaster command to completion and returns its
// output and exit code.
func runPatchcaster(t *testing.T, args ...string) (string, int) {
	t.Helper()
	out, err := exec.Command(binaryPath, args...).CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr, "run patchcaster %v", args)
		return string(out), exitErr.ExitCode()
	}
	return string(out), 0
}

// stop ends a running service the way an orchestrator does.
func (s *service) stop(t *testing.T) {
	t.Helper()
	s.signal(t, syscall.SIGTERM)
	require.Zero(t, s.awaitExit(t, 30*time.Second), "a drained service exits cleanly")
}

// A restart with another key is refused, and neither key reaches the logs
// (SYSREQ-042).
func TestAnotherEncryptionKeyIsRefused(t *testing.T) {
	// arrange
	freshDatabase(t)
	startService(t, serviceConfig{}).stop(t)

	// act
	svc := launch(t, serviceConfig{encryptionKey: rotatedKey})
	code := svc.awaitExit(t, 15*time.Second)

	// assert
	logs := svc.logs.String()
	assert.NotZero(t, code)
	assert.Contains(t, logs, "the encryption key does not open this database's secrets")
	assert.NotContains(t, logs, suiteKey)
	assert.NotContains(t, logs, rotatedKey)
}

// A key rotates without downtime: the new key with the old one as a previous
// key, a reseal, then the new key on its own (ADR-035).
func TestTheEncryptionKeyRotates(t *testing.T) {
	// arrange
	freshDatabase(t)
	startService(t, serviceConfig{}).stop(t)

	// act
	during := startService(t, serviceConfig{
		encryptionKey: rotatedKey,
		previousKeys:  []string{suiteKey},
	})
	during.stop(t)
	resealOut, resealCode := runPatchcaster(t, "secrets", "reseal",
		"--database-url", databaseURL,
		"--encryption-key", rotatedKey,
		"--previous-encryption-key", suiteKey)
	after := startService(t, serviceConfig{encryptionKey: rotatedKey})

	// assert
	assert.Contains(t, during.logs.String(), "run patchcaster secrets reseal")
	assert.Zero(t, resealCode, resealOut)
	assert.Contains(t, resealOut, "count=1")
	assert.NotContains(t, after.logs.String(), "secrets reseal", "after the reseal the new key needs no previous one")
}
