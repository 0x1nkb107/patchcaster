package e2e

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// serviceConfig tunes one patchcaster process. Zero values take the suite
// defaults below.
type serviceConfig struct {
	mattermostURL string
	pollInterval  time.Duration
	claimLease    time.Duration
	drainTimeout  time.Duration
}

// Suite defaults: a short poll so that retries come due quickly, a short
// lease so that a killed delivery is reaped quickly, and the production
// drain budget.
const (
	defaultPollInterval = 200 * time.Millisecond
	defaultClaimLease   = 2 * time.Second
	defaultDrainTimeout = 30 * time.Second
)

func (c serviceConfig) withDefaults() serviceConfig {
	if c.pollInterval == 0 {
		c.pollInterval = defaultPollInterval
	}
	if c.claimLease == 0 {
		c.claimLease = defaultClaimLease
	}
	if c.drainTimeout == 0 {
		c.drainTimeout = defaultDrainTimeout
	}
	return c
}

// service is one running patchcaster process.
type service struct {
	cmd         *exec.Cmd
	publicAddr  string
	internalURL string
	gitlab      gitlab
	logs        *lockedBuffer
	exited      chan struct{}
}

// startService starts patchcaster against the suite database and waits until
// it reports ready. The process is killed when the test ends, and its logs
// are printed when the test fails.
func startService(t *testing.T, cfg serviceConfig) *service {
	t.Helper()
	cfg = cfg.withDefaults()
	publicAddr, internalAddr := freeAddress(t), freeAddress(t)

	logs := &lockedBuffer{}
	cmd := exec.Command(binaryPath, "serve",
		"--addr", publicAddr,
		"--internal-addr", internalAddr,
		"--database-url", databaseURL,
		"--mattermost-url", cfg.mattermostURL,
		"--mattermost-token", botToken,
		"--signing-secret", signingSecret,
		"--routing-file", writeRoutingFile(t),
		"--poll-interval", cfg.pollInterval.String(),
		"--claim-lease", cfg.claimLease.String(),
		"--drain-timeout", cfg.drainTimeout.String(),
	)
	cmd.Stdout = logs
	cmd.Stderr = logs
	require.NoError(t, cmd.Start(), "start patchcaster")

	s := &service{
		cmd:         cmd,
		publicAddr:  publicAddr,
		internalURL: "http://" + internalAddr,
		gitlab:      gitlab{webhookURL: "http://" + publicAddr + "/webhook"},
		logs:        logs,
		exited:      make(chan struct{}),
	}
	go func() {
		_ = cmd.Wait()
		close(s.exited)
	}()
	t.Cleanup(func() { s.cleanUp(t) })

	eventually(t, 15*time.Second, "the service to report ready", func() bool {
		return s.readiness() == http.StatusOK
	})
	return s
}

// cleanUp kills a process the test left running and shows its logs if the
// test failed.
func (s *service) cleanUp(t *testing.T) {
	select {
	case <-s.exited:
	default:
		_ = s.cmd.Process.Kill()
		<-s.exited
	}
	if t.Failed() {
		t.Logf("patchcaster logs:\n%s", s.logs.String())
	}
}

// signal sends a signal to the process, as an orchestrator would.
func (s *service) signal(t *testing.T, sig syscall.Signal) {
	t.Helper()
	require.NoError(t, s.cmd.Process.Signal(sig))
}

// awaitExit waits for the process to end and returns its exit code, which
// is -1 when a signal killed it.
func (s *service) awaitExit(t *testing.T, within time.Duration) int {
	t.Helper()
	select {
	case <-s.exited:
		return s.cmd.ProcessState.ExitCode()
	case <-time.After(within):
		t.Fatalf("patchcaster did not exit within %s", within)
		return 0
	}
}

// readiness returns the status code of the readiness probe, or 0 when the
// internal listener does not answer.
func (s *service) readiness() int {
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(s.internalURL + "/readyz")
	if err != nil {
		return 0
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// listening reports whether the wake-up listener has a confirmed session,
// according to the service's own gauge.
func (s *service) listening() bool {
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(s.internalURL + "/metrics")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	metrics, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	return strings.Contains(string(metrics), "\npatchcaster_outbox_listener_up 1\n")
}

// acceptsConnections reports whether the webhook listener accepts a TCP
// connection.
func (s *service) acceptsConnections() bool {
	conn, err := net.DialTimeout("tcp", s.publicAddr, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// writeRoutingFile routes the suite's project to the suite's channel.
func writeRoutingFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routing.yaml")
	routes := "routes:\n  - project: " + projectPath + "\n    channel: " + channelID + "\n"
	require.NoError(t, os.WriteFile(path, []byte(routes), 0o600))
	return path
}

// freeAddress finds a loopback address that nothing listens on right now.
func freeAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// eventually polls cond until it holds and fails the test once within has
// passed. The limit only guards against a hang: a healthy service meets each
// condition within milliseconds to a few seconds.
func eventually(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", within, what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// lockedBuffer collects process output that two pipes write concurrently.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
