package ingest

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/metrics"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

var (
	fixedNow = time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)
	testKey  = []byte("kms-test")
)

// --- fake store -------------------------------------------------------------

type acceptRec struct {
	ev         *event.Event
	deliveries []store.Delivery
}

type outcomeRec struct {
	id      string
	outcome string
}

type rejectionRec struct {
	connectionID int64
	cause        string
}

type fakeStore struct {
	seen         map[string]bool
	destinations []store.Destination // what every event of testConnection reaches
	accepted     []acceptRec
	outcomes     []outcomeRec
	errSeen      error
	errAccept    error
	errMark      error
	acceptDup    bool // simulate losing the seen-insert race

	// The health the handler records for forge connections.
	delivered []int64
	rejected  []rejectionRec
	errHealth error
}

// testConnection is the forge connection whose endpoint the tests post to,
// and testLabel the label its metrics carry.
const (
	testConnection = 7
	testLabel      = "7"
)

// testEndpoint is that connection's webhook endpoint.
const testEndpoint = "/webhooks/7"

func newFakeStore() *fakeStore {
	return &fakeStore{
		seen: map[string]bool{},
		destinations: []store.Destination{{
			SinkID:       1,
			ConnectionID: 2,
			ChannelID:    "chan-1",
			MutedReplies: []string{},
		}},
	}
}

func (f *fakeStore) Destinations(_ context.Context, connectionID int64, repository string) ([]store.Destination, error) {
	if connectionID != testConnection || repository != "grp/app" {
		return nil, nil
	}
	return f.destinations, nil
}

func (f *fakeStore) Seen(_ context.Context, id string) (bool, error) {
	if f.errSeen != nil {
		return false, f.errSeen
	}
	return f.seen[id], nil
}

func (f *fakeStore) MarkOutcome(_ context.Context, id, outcome string, _ time.Time) error {
	if f.errMark != nil {
		return f.errMark
	}
	f.outcomes = append(f.outcomes, outcomeRec{id: id, outcome: outcome})
	return nil
}

func (f *fakeStore) Accept(_ context.Context, ev *event.Event, deliveries []store.Delivery, _ time.Time) (store.AcceptResult, error) {
	if f.errAccept != nil {
		return store.AcceptResult{}, f.errAccept
	}
	if f.acceptDup {
		return store.AcceptResult{}, nil
	}
	f.accepted = append(f.accepted, acceptRec{ev: ev, deliveries: deliveries})
	return store.AcceptResult{Accepted: true, Enqueued: len(deliveries), Attributed: 1}, nil
}

func (f *fakeStore) RecordForgeDelivery(_ context.Context, connectionID int64, _ time.Time) error {
	if f.errHealth != nil {
		return f.errHealth
	}
	f.delivered = append(f.delivered, connectionID)
	return nil
}

func (f *fakeStore) RecordForgeRejection(_ context.Context, connectionID int64, _ time.Time, cause string) error {
	if f.errHealth != nil {
		return f.errHealth
	}
	f.rejected = append(f.rejected, rejectionRec{connectionID: connectionID, cause: cause})
	return nil
}

// fakeKeys holds the signing key of testConnection only.
type fakeKeys struct{}

func (fakeKeys) SigningKeys(_ context.Context, connectionID int64) ([][]byte, error) {
	if connectionID != testConnection {
		return nil, ErrUnknownEndpoint
	}
	return [][]byte{testKey}, nil
}

// --- helpers ----------------------------------------------------------------

func newHandler(t *testing.T, fs Store) (*Handler, *metrics.Metrics) {
	t.Helper()
	m := metrics.New()
	return &Handler{
		Keys:    fakeKeys{},
		Store:   fs,
		MaxAge:  4 * time.Hour,
		Now:     func() time.Time { return fixedNow },
		Log:     slog.New(slog.DiscardHandler),
		Metrics: m,
	}, m
}

// deliver signs body the way GitLab's Standard Webhooks client would and
// posts it to the mux.
func deliver(t *testing.T, h http.Handler, id string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	ts := "1760090000"
	mac := hmac.New(sha256.New, testKey)
	_, err := mac.Write([]byte(id + "." + ts + "." + string(body)))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, testEndpoint, bytes.NewReader(body))
	req.Header.Set("Webhook-Id", id)
	req.Header.Set("Webhook-Timestamp", ts)
	req.Header.Set("Webhook-Signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func mrBody(t *testing.T, action, timestamp string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"object_kind": "merge_request",
		"event_name":  "merge_request",
		"user":        map[string]any{"username": "ada"},
		"project":     map[string]any{"id": 42, "full_path": "grp/app", "path_with_namespace": "grp/app"},
		"object_attributes": map[string]any{
			"id": 987654321, "iid": 12, "title": "Add flux",
			"url":    "https://gl.example/grp/app/-/merge_requests/12",
			"action": action, "timestamp": timestamp,
		},
	})
	require.NoError(t, err)
	return body
}

const freshTS = "2026-10-10T09:00:00.123Z" // one hour before fixedNow

// --- decision table ----------------------------------------------------------

func TestHandler_AcceptsFreshOpenEvent(t *testing.T) {
	// arrange
	fs := newFakeStore()
	h, m := newHandler(t, fs)

	// act
	rec := deliver(t, h.Mux(), "msg-1", mrBody(t, "open", freshTS))

	// assert
	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, fs.accepted, 1)
	ev := fs.accepted[0].ev
	assert.Equal(t, "msg-1", ev.WebhookID)
	assert.Equal(t, "7:987654321", ev.MRRef, "the key names the connection")
	assert.Equal(t, int64(testConnection), ev.ConnectionID)
	assert.Equal(t, "open", ev.Action)
	require.Len(t, fs.accepted[0].deliveries, 1)
	assert.Equal(t, "chan-1", fs.accepted[0].deliveries[0].ChannelID)
	assert.True(t, fs.accepted[0].deliveries[0].Plan.CreateRoot)
	assert.True(t, ev.When.Equal(time.Date(2026, 10, 10, 9, 0, 0, 123_000_000, time.UTC)),
		"event timestamp must survive ms-precision parsing, got %s", ev.When)
	assert.Equal(t, 1.0, testutil.ToFloat64(m.Received.WithLabelValues(testLabel)))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.StateApplied.WithLabelValues(event.KindMergeRequest, testLabel)))
	assert.Equal(t, []int64{testConnection}, fs.delivered, "the connection's health shows the delivery")
}

func TestHandler_SuppressesSeenDelivery(t *testing.T) {
	// arrange
	fs := newFakeStore()
	fs.seen["msg-1"] = true
	h, m := newHandler(t, fs)

	// act
	rec := deliver(t, h.Mux(), "msg-1", mrBody(t, "open", freshTS))

	// assert
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, fs.accepted)
	assert.Equal(t, 1.0, testutil.ToFloat64(m.Suppressed.WithLabelValues(testLabel)))
}

func TestHandler_RejectsBadSignature(t *testing.T) {
	// arrange
	fs := newFakeStore()
	h, m := newHandler(t, fs)
	body := mrBody(t, "open", freshTS)

	// act: sign with an unknown key.
	mac := hmac.New(sha256.New, []byte("attacker"))
	_, err := mac.Write([]byte("msg-1.1760090000." + string(body)))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, testEndpoint, bytes.NewReader(body))
	req.Header.Set("Webhook-Id", "msg-1")
	req.Header.Set("Webhook-Timestamp", "1760090000")
	req.Header.Set("Webhook-Signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	rec := httptest.NewRecorder()
	h.Mux().ServeHTTP(rec, req)

	// assert
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, fs.accepted)
	assert.Equal(t, 1.0, testutil.ToFloat64(m.Rejected.WithLabelValues(metrics.CauseSignature, testLabel)))
	assert.Equal(t, []rejectionRec{{connectionID: testConnection, cause: metrics.CauseSignature}}, fs.rejected,
		"the connection's health shows the rejection")
	assert.Empty(t, fs.delivered, "an unsigned delivery is no delivery of the connection")
}

func TestHandler_RejectsUnparseableBody(t *testing.T) {
	// arrange
	fs := newFakeStore()
	h, m := newHandler(t, fs)

	// act
	rec := deliver(t, h.Mux(), "msg-1", []byte("this is not json"))

	// assert
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, fs.outcomes) // a 4xx leaves no seen mark; GitLab may retry
	assert.Equal(t, 1.0, testutil.ToFloat64(m.Rejected.WithLabelValues(metrics.CauseParse, testLabel)))
	assert.Equal(t, []rejectionRec{{connectionID: testConnection, cause: metrics.CauseParse}}, fs.rejected)
}

func TestHandler_Discards(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		cause   string
		outcome string
	}{
		{
			name:    "expired",
			body:    mrBody(t, "open", "2026-10-10T05:00:00.123Z"), // five hours old
			cause:   metrics.CauseExpired,
			outcome: metrics.CauseExpired,
		},
		{
			name:    "unparseable timestamp counts as expired",
			body:    mrBody(t, "open", "not-a-timestamp"),
			cause:   metrics.CauseExpired,
			outcome: metrics.CauseExpired,
		},
		{
			name:  "unprocessed kind",
			body:  []byte(`{"object_kind":"wiki_page"}`),
			cause: metrics.CauseUnprocessed, outcome: metrics.CauseUnprocessed,
		},
		{
			name:  "note on non-merge-request names nothing",
			body:  []byte(`{"object_kind":"note","user":{"username":"bob"},"project":{"full_path":"grp/app"},"object_attributes":{"id":9,"noteable_type":"Issue","action":"create","created_at":"` + freshTS + `"}}`),
			cause: metrics.CauseUnlinked, outcome: metrics.CauseUnlinked,
		},
		{
			name:  "branch pipeline names nothing",
			body:  []byte(`{"object_kind":"pipeline","project":{"full_path":"grp/app"},"object_attributes":{"id":31,"status":"running","ref":"main","created_at":"` + freshTS + `"}}`),
			cause: metrics.CauseUnlinked, outcome: metrics.CauseUnlinked,
		},
		{
			name:  "tag push names no branch",
			body:  []byte(`{"object_kind":"push","ref":"refs/tags/v1","project":{"id":42,"full_path":"grp/app"},"total_commits_count":0}`),
			cause: metrics.CauseUnlinked, outcome: metrics.CauseUnlinked,
		},
		{
			name:    "action not allowlisted",
			body:    mrBody(t, "totally_new_gitlab_action", freshTS),
			cause:   metrics.CauseNotAllowlisted,
			outcome: metrics.CauseNotAllowlisted,
		},
		{
			name:    "unrouted project",
			body:    unroutedBody(t),
			cause:   metrics.CauseUnrouted,
			outcome: metrics.CauseUnrouted,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			fs := newFakeStore()
			h, m := newHandler(t, fs)

			// act
			rec := deliver(t, h.Mux(), "msg-x", tc.body)

			// assert
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Empty(t, fs.accepted, "discarded deliveries must not enqueue")
			require.Len(t, fs.outcomes, 1, "the seen mark must carry the decision")
			assert.Equal(t, tc.outcome, fs.outcomes[0].outcome)
			assert.Equal(t, 1.0, testutil.ToFloat64(m.Discarded.WithLabelValues(tc.cause, testLabel)))
		})
	}
}

func unroutedBody(t *testing.T) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"object_kind": "merge_request",
		"user":        map[string]any{"username": "ada"},
		"project":     map[string]any{"full_path": "grp/other"},
		"object_attributes": map[string]any{
			"id": 7, "iid": 1, "action": "open", "timestamp": freshTS,
		},
	})
	require.NoError(t, err)
	return body
}

func TestHandler_StoreFailuresNeverAck(t *testing.T) {
	tests := []struct {
		name string
		fail func(*fakeStore)
	}{
		{"dedupe lookup down", func(fs *fakeStore) { fs.errSeen = errors.New("down") }},
		{"accept down", func(fs *fakeStore) { fs.errAccept = errors.New("down") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			fs := newFakeStore()
			tc.fail(fs)
			h, _ := newHandler(t, fs)

			// act
			rec := deliver(t, h.Mux(), "msg-1", mrBody(t, "open", freshTS))

			// assert: a 5xx invites the GitLab retry; a 2xx would lie.
			assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		})
	}
}

func TestHandler_MarkFailureNeverAck(t *testing.T) {
	// arrange: expired event, store rejects the seen mark.
	fs := newFakeStore()
	fs.errMark = errors.New("down")
	h, _ := newHandler(t, fs)

	// act
	rec := deliver(t, h.Mux(), "msg-1", mrBody(t, "open", "2026-10-09T05:00:00.123Z"))

	// assert
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandler_AcceptRaceSuppresses(t *testing.T) {
	// arrange: the concurrent sibling inserted the seen row first.
	fs := newFakeStore()
	fs.acceptDup = true
	h, m := newHandler(t, fs)

	// act
	rec := deliver(t, h.Mux(), "msg-1", mrBody(t, "open", freshTS))

	// assert
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, fs.accepted)
	assert.Equal(t, 1.0, testutil.ToFloat64(m.Suppressed.WithLabelValues(testLabel)))
}

func TestHandler_HealthIsNoReasonToRefuseADelivery(t *testing.T) {
	// arrange
	fs := newFakeStore()
	fs.errHealth = errors.New("down")
	h, _ := newHandler(t, fs)

	// act
	rec := deliver(t, h.Mux(), "msg-1", mrBody(t, "open", freshTS))

	// assert
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Len(t, fs.accepted, 1)
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	// arrange
	fs := newFakeStore()
	h, m := newHandler(t, fs)

	// act
	rec := httptest.NewRecorder()
	h.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, testEndpoint, nil))

	// assert
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, 0, testutil.CollectAndCount(m.Received), "non-POST traffic is not a delivery")
}

func TestHandler_RefusesAnEndpointNoForgeOwns(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"an unknown connection", "/webhooks/8"},
		{"no connection id", "/webhooks/gitlab"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			fs := newFakeStore()
			h, m := newHandler(t, fs)
			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(mrBody(t, "open", freshTS)))

			// act
			rec := httptest.NewRecorder()
			h.Mux().ServeHTTP(rec, req)

			// assert
			assert.Equal(t, http.StatusNotFound, rec.Code)
			assert.Empty(t, fs.accepted)
			assert.Equal(t, 1.0, testutil.ToFloat64(m.Received.WithLabelValues("")), "an unknown endpoint counts under no connection")
			assert.Equal(t, 1.0, testutil.ToFloat64(m.Rejected.WithLabelValues(metrics.CauseEndpoint, "")))
			assert.Empty(t, fs.rejected, "no connection's health shows it")
		})
	}
}

func commentBody(t *testing.T) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"object_kind": "note",
		"user":        map[string]any{"username": "bob"},
		"project":     map[string]any{"id": 42, "full_path": "grp/app", "path_with_namespace": "grp/app"},
		"object_attributes": map[string]any{
			"action": "create", "noteable_type": "MergeRequest", "note": "looks good",
			"created_at": freshTS,
		},
		"merge_request": map[string]any{"id": 987654321, "iid": 12},
	})
	require.NoError(t, err)
	return body
}

// A sink gets one delivery however many routes reach it, and its reply
// unless every one of those routes mutes it (SYSREQ-046, SYSREQ-047).
func TestHandler_MergesTheRoutesOfASink(t *testing.T) {
	tests := []struct {
		name       string
		muted      [][]string // per route to the one sink
		wantNotice string
	}{
		{"one route mutes, the other lets it through", [][]string{{"comment"}, {}}, "comment"},
		{"every route mutes", [][]string{{"comment", "push"}, {"comment"}}, ""},
		{"no route mutes", [][]string{{}}, "comment"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			fs := newFakeStore()
			fs.destinations = nil
			for _, muted := range tc.muted {
				fs.destinations = append(fs.destinations, store.Destination{
					SinkID:       1,
					ConnectionID: 2,
					ChannelID:    "chan-1",
					MutedReplies: muted,
				})
			}
			h, _ := newHandler(t, fs)

			// act
			rec := deliver(t, h.Mux(), "msg-c", commentBody(t))

			// assert
			require.Equal(t, http.StatusOK, rec.Code)
			require.Len(t, fs.accepted, 1)
			require.Len(t, fs.accepted[0].deliveries, 1, "one sink, one delivery")
			assert.Equal(t, tc.wantNotice, fs.accepted[0].deliveries[0].Plan.Notice)
		})
	}
}

func TestHandler_FansOutToEverySink(t *testing.T) {
	// arrange
	fs := newFakeStore()
	fs.destinations = []store.Destination{
		{SinkID: 1, ConnectionID: 2, ChannelID: "chan-1", MutedReplies: []string{"comment"}},
		{SinkID: 3, ConnectionID: 4, ChannelID: "room-3", MutedReplies: []string{}},
	}
	h, _ := newHandler(t, fs)

	// act
	rec := deliver(t, h.Mux(), "msg-c", commentBody(t))

	// assert
	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, fs.accepted, 1)
	deliveries := fs.accepted[0].deliveries
	require.Len(t, deliveries, 2)
	assert.Empty(t, deliveries[0].Plan.Notice, "the first sink mutes comments")
	assert.Equal(t, "comment", deliveries[1].Plan.Notice)
	assert.Equal(t, "comment", fs.accepted[0].ev.Plan.Notice, "the event keeps its own plan")
}
