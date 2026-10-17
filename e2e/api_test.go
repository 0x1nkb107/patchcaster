package e2e

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The management API answers, and it serves the description it was built
// from (SYSREQ-035).
func TestTheManagementAPIDescribesTheInstance(t *testing.T) {
	// arrange
	freshDatabase(t)
	svc := startService(t, serviceConfig{})
	want, err := os.ReadFile("../internal/api/openapi-v1.yaml")
	require.NoError(t, err)

	// act
	instance, instanceErr := newBrowser(t, svc).api(t).GetInstance(context.Background())
	resp, getErr := http.Get(svc.managementURL + "/api/v1/openapi.yaml")
	require.NoError(t, getErr)
	defer func() { _ = resp.Body.Close() }()
	served, readErr := io.ReadAll(resp.Body)

	// assert
	require.NoError(t, instanceErr)
	require.NoError(t, readErr)
	assert.Equal(t, "dev", instance.Version, "a source build reports dev")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, string(want), string(served))
}

// Each listener answers only its own paths, so platforms can expose each
// one differently (SYSREQ-036).
func TestEachListenerServesOnlyItsOwnPaths(t *testing.T) {
	// arrange
	freshDatabase(t)
	svc := startService(t, serviceConfig{})
	webhookURL := "http://" + svc.publicAddr

	tests := []struct {
		name       string
		method     string
		url        string
		wantStatus int
		wantType   string // the content type, when the listener answers at all
	}{
		{"the API on the webhook listener", http.MethodGet, webhookURL + "/api/v1/instance", http.StatusNotFound, ""},
		{"the API on the internal listener", http.MethodGet, svc.internalURL + "/api/v1/instance", http.StatusNotFound, ""},
		{"the interface on the webhook listener", http.MethodGet, webhookURL + "/", http.StatusNotFound, ""},
		// The interface routes in the browser, so the management listener
		// answers every other read with its page and refuses every write.
		{"webhooks on the management listener", http.MethodPost, svc.managementURL + "/webhooks/1", http.StatusMethodNotAllowed, ""},
		{"probes on the management listener", http.MethodGet, svc.managementURL + "/readyz", http.StatusOK, "text/html; charset=utf-8"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			req, err := http.NewRequest(tc.method, tc.url, strings.NewReader("{}"))
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()

			// assert
			assert.Equal(t, tc.wantStatus, resp.StatusCode)
			if tc.wantType != "" {
				assert.Equal(t, tc.wantType, resp.Header.Get("Content-Type"), "the interface's page, not the probe")
			}
		})
	}
}

// The binary serves its web interface on the management listener, and the
// interface's router takes every path that names no file (ADR-032).
func TestTheManagementListenerServesTheInterface(t *testing.T) {
	// arrange
	freshDatabase(t)
	svc := startService(t, serviceConfig{})
	read := func(path string) (int, http.Header, string) {
		resp, err := http.Get(svc.managementURL + path)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, resp.Header, string(body)
	}

	// act
	rootStatus, rootHeader, root := read("/")
	deepStatus, _, deep := read("/tokens")

	// assert
	assert.Equal(t, http.StatusOK, rootStatus)
	assert.Equal(t, "no-cache", rootHeader.Get("Cache-Control"), "the page names the release's files, so browsers check it every time")
	assert.Contains(t, rootHeader.Get("Content-Security-Policy"), "frame-ancestors 'none'", "no other site may frame the interface")
	assert.Equal(t, http.StatusOK, deepStatus)
	assert.Equal(t, root, deep, "a deep link gets the same page")
	assert.Contains(t, root, "<title>PatchCaster</title>")
}
