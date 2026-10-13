package e2e

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The service is started with this secret and routes this project to this
// channel.
const (
	signingSecret = "e2e-signing-secret"
	projectPath   = "group/application"
	channelID     = "chan-e2e"
)

// mergeRequest identifies a merge request in the payloads.
type mergeRequest struct {
	id  int // GitLab's global id, which the service keys threads by
	iid int // the number within the project
}

// ref is the key the store uses for the merge request.
func (mr mergeRequest) ref() string { return strconv.Itoa(mr.id) }

// gitlab sends webhook deliveries the way GitLab does: JSON bodies signed
// under the Standard Webhooks scheme.
type gitlab struct {
	webhookURL string
}

// deliver sends body under a fresh delivery id and returns the status code.
func (g gitlab) deliver(t *testing.T, body []byte) int {
	t.Helper()
	return g.deliverAs(t, newDeliveryID(t), body)
}

// deliverAs sends body under the given delivery id, signed with the shared
// secret, and returns the status code.
func (g gitlab) deliverAs(t *testing.T, id string, body []byte) int {
	t.Helper()
	return g.deliverSigned(t, id, body, signingSecret)
}

// deliverSigned sends body signed with secret and returns the status code.
func (g gitlab) deliverSigned(t *testing.T, id string, body []byte, secret string) int {
	t.Helper()
	resp, err := g.post(id, body, secret)
	require.NoError(t, err, "send webhook")
	_ = resp.Body.Close()
	return resp.StatusCode
}

// post sends one signed delivery and leaves error handling to the caller.
func (g gitlab) post(id string, body []byte, secret string) (*http.Response, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req, err := http.NewRequest(http.MethodPost, g.webhookURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Webhook-Id", id)
	req.Header.Set("Webhook-Timestamp", timestamp)
	req.Header.Set("Webhook-Signature", sign(secret, id, timestamp, body))

	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

// sign computes the Standard Webhooks signature: base64 HMAC-SHA256 over
// "id.timestamp.body", prefixed with the scheme version.
func sign(secret, id, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(id + "." + timestamp + "."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func newDeliveryID(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 16)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	return hex.EncodeToString(raw)
}

// gitlabTime formats a moment the way GitLab 19 writes timestamps.
func gitlabTime(at time.Time) string {
	return at.UTC().Format("2006-01-02T15:04:05.000Z")
}

func projectFields() map[string]any {
	return map[string]any{"id": 9, "full_path": projectPath, "path_with_namespace": projectPath}
}

// mergeRequestOpened is the payload GitLab sends when a merge request opens.
func mergeRequestOpened(t *testing.T, mr mergeRequest) []byte {
	t.Helper()
	return payload(t, map[string]any{
		"object_kind": "merge_request",
		"event_name":  "merge_request",
		"project":     projectFields(),
		"user":        map[string]any{"id": 1, "username": "alice"},
		"object_attributes": map[string]any{
			"id":            mr.id,
			"iid":           mr.iid,
			"title":         "Add token refresh",
			"description":   "Refreshes OAuth tokens proactively.",
			"source_branch": "feature/auth",
			"target_branch": "main",
			"action":        "open",
			"state":         "opened",
			"url":           "https://gitlab.example/" + projectPath + "/-/merge_requests/" + strconv.Itoa(mr.iid),
			"timestamp":     gitlabTime(time.Now()),
		},
		"reviewers": []any{},
	})
}

// commentCreated is the payload GitLab sends for a new human comment on a
// merge request.
func commentCreated(t *testing.T, mr mergeRequest, noteID int) []byte {
	t.Helper()
	return payload(t, map[string]any{
		"object_kind": "note",
		"event_name":  "note",
		"project":     projectFields(),
		"user":        map[string]any{"id": 3, "username": "bob"},
		"object_attributes": map[string]any{
			"id":            noteID,
			"action":        "create",
			"noteable_type": "MergeRequest",
			"note":          "Ship it.",
			"created_at":    gitlabTime(time.Now()),
		},
		"merge_request": map[string]any{"id": mr.id, "iid": mr.iid},
	})
}

func payload(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(fields)
	require.NoError(t, err)
	return body
}
