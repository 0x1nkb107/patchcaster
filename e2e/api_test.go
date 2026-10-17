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
		name   string
		method string
		url    string
	}{
		{"the API on the webhook listener", http.MethodGet, webhookURL + "/api/v1/instance"},
		{"the API on the internal listener", http.MethodGet, svc.internalURL + "/api/v1/instance"},
		{"webhooks on the management listener", http.MethodPost, svc.managementURL + "/webhook"},
		{"probes on the management listener", http.MethodGet, svc.managementURL + "/readyz"},
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
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	}
}
