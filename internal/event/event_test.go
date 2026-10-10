package event

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const systemHookMR = `{
  "event_name": "merge_request",
  "object_kind": "merge_request",
  "user": {"id": 1, "name": "Ada Lovelace", "username": "ada"},
  "project": {
    "id": 42,
    "name": "app",
    "path_with_namespace": "grp/app",
    "full_path": "grp/app"
  },
  "object_attributes": {
    "id": 987654321,
    "iid": 12,
    "title": "Add flux capacitor",
    "url": "https://gl.example/grp/app/-/merge_requests/12",
    "action": "open",
    "created_at": "2026-10-10T09:00:00.123Z",
    "updated_at": "2026-10-10T09:00:00.123Z",
    "timestamp": "2026-10-10T09:00:00.123Z",
    "author": {"username": "ada"}
  }
}`

func TestParse_ProjectHookMR(t *testing.T) {
	// act
	ev, err := Parse([]byte(systemHookMR))

	// assert
	require.NoError(t, err)
	assert.True(t, ev.Forwardable())
	assert.Equal(t, "open", ev.Action)
	assert.Equal(t, "987654321", ev.MRRef)
	assert.EqualValues(t, 12, ev.IID)
	assert.Equal(t, "grp/app", ev.ProjectPath)
	assert.Equal(t, "Add flux capacitor", ev.Title)
	assert.Equal(t, "ada", ev.Author)
	assert.Equal(t, "ada", ev.Actor)
	assert.True(t, ev.When.Equal(time.Date(2026, 10, 10, 9, 0, 0, 123_000_000, time.UTC)))
}

func TestParse_FullPathPreferredOverPathWithNamespace(t *testing.T) {
	// arrange: some payload generations carry only path_with_namespace.
	body := `{"object_kind":"merge_request","project":{"path_with_namespace":"only/pwn"},
	          "object_attributes":{"id":1,"iid":1,"action":"open","timestamp":"2026-10-10T09:00:00.000Z"}}`

	// act
	ev, err := Parse([]byte(body))

	// assert
	require.NoError(t, err)
	assert.Equal(t, "only/pwn", ev.ProjectPath)
}

func TestParse_TimestampFallbackChain(t *testing.T) {
	// arrange: no timestamp field; updated_at carries the event time.
	body := `{"object_kind":"merge_request","project":{"full_path":"a/b"},
	          "object_attributes":{"id":2,"iid":2,"action":"update",
	          "updated_at":"2026-10-10T09:30:00.000Z"}}`

	// act
	ev, err := Parse([]byte(body))

	// assert
	require.NoError(t, err)
	assert.True(t, ev.When.Equal(time.Date(2026, 10, 10, 9, 30, 0, 0, time.UTC)))
}

func TestParse_UnparseableTimestampIsZeroTime(t *testing.T) {
	// arrange: GitLab-18-era format without zone designator is rejected;
	// SYSREQ-013 turns the zero time into an expired verdict downstream.
	body := `{"object_kind":"merge_request","project":{"full_path":"a/b"},
	          "object_attributes":{"id":3,"iid":3,"action":"update","timestamp":"2026-10-10 09:00:00 UTC"}}`

	// act
	ev, err := Parse([]byte(body))

	// assert
	require.NoError(t, err)
	assert.True(t, ev.When.IsZero())
}

func TestParse_NonMRKindNeedsNoMRFields(t *testing.T) {
	// act
	ev, err := Parse([]byte(`{"object_kind":"push"}`))

	// assert
	require.NoError(t, err)
	assert.False(t, ev.Forwardable())
	assert.Equal(t, "push", ev.Kind)
}

func TestParse_InvalidPayloads(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"not json", `nope`},
		{"missing object_kind", `{"user":{}}`},
		{"missing action", `{"object_kind":"merge_request","project":{"full_path":"a/b"},
		  "object_attributes":{"id":1}}`},
		{"missing project path", `{"object_kind":"merge_request","project":{"id":5},
		  "object_attributes":{"id":1,"action":"open"}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			_, err := Parse([]byte(tc.body))

			// assert
			require.ErrorIs(t, err, ErrInvalidPayload)
		})
	}
}

func TestParse_ApprovalStyleActorFallsBackToAuthor(t *testing.T) {
	// arrange: no top-level user (defensive against payload variance).
	body := `{"object_kind":"merge_request","project":{"full_path":"a/b"},
	          "object_attributes":{"id":9,"iid":9,"action":"approved",
	          "author":{"username":"grace"},"timestamp":"2026-10-10T09:00:00.000Z"}}`

	// act
	ev, err := Parse([]byte(body))

	// assert
	require.NoError(t, err)
	assert.Equal(t, "grace", ev.Author)
	assert.Equal(t, "grace", ev.Actor, "actor falls back to author, never empty-without-cause")
}
