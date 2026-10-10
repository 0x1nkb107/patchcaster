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

func TestParse_MRStateInputs(t *testing.T) {
	body := `{
  "object_kind": "merge_request",
  "user": {"id": 7, "username": "carol"},
  "project": {"id": 42, "full_path": "grp/app"},
  "object_attributes": {
    "id": 987654321, "iid": 12, "action": "update", "state": "opened",
    "source_branch": "feature/auth", "source_project_id": 42,
    "oldrev": "e59094b8de0f2f91abbe4760a52d9137260252d8",
    "system": false,
    "approval_rules": [{"approvals_required": 1}, {"approvals_required": 1}],
    "updated_at": "2026-10-10T09:05:00.000Z",
    "author": {"username": "ada"}
  },
  "reviewers": [
    {"id": 6, "username": "bob", "state": "reviewed"},
    {"id": 7, "username": "carol", "state": "unreviewed"}
  ]
}`

	// act
	ev, err := Parse([]byte(body))

	// assert
	require.NoError(t, err)
	assert.True(t, ev.Open)
	assert.Equal(t, "feature/auth", ev.SourceBranch)
	assert.EqualValues(t, 42, ev.SourceProjectID)
	assert.True(t, ev.Pushed)
	assert.True(t, ev.EditChanged, "a push counts as one edit")
	assert.Equal(t, 1, ev.ReviewersDone)
	assert.Equal(t, 2, ev.ReviewersRequired, "approval-rule minimum wins")
	assert.Empty(t, ev.ReviewState, "no reviewer state transitioned here")
}

func TestParse_ReviewTransitionIsANotice(t *testing.T) {
	body := `{
  "object_kind": "merge_request",
  "user": {"id": 7, "username": "carol"},
  "project": {"id": 42, "full_path": "grp/app"},
  "object_attributes": {
    "id": 987654321, "iid": 12, "action": "update", "state": "opened",
    "source_branch": "feature/auth", "source_project_id": 42,
    "updated_at": "2026-10-10T09:05:00.000Z"
  },
  "reviewers": [{"id": 7, "username": "carol", "state": "requested_changes"}],
  "changes": {"reviewers": [
    [{"id": 7, "state": "review_started"}],
    [{"id": 7, "state": "requested_changes"}]
  ]}
}`

	// act
	ev, err := Parse([]byte(body))

	// assert
	require.NoError(t, err)
	assert.Equal(t, ReviewChanges, ev.ReviewState)
	assert.False(t, ev.EditChanged, "a review is not an edit")
}

func TestParse_ReviewerDenominatorFallsBackToAssigned(t *testing.T) {
	body := `{
  "object_kind": "merge_request",
  "user": {"id": 1, "username": "ada"},
  "project": {"id": 42, "full_path": "grp/app"},
  "object_attributes": {"id": 1, "iid": 1, "action": "open", "state": "opened",
    "approval_rules": [{"approvals_required": 0, "rule_type": "any_approver"}]},
  "reviewers": [{"id": 6, "state": "approved"}, {"id": 7, "state": "unreviewed"}]
}`

	// act
	ev, err := Parse([]byte(body))

	// assert
	require.NoError(t, err)
	assert.Equal(t, 1, ev.ReviewersDone)
	assert.Equal(t, 2, ev.ReviewersRequired, "zero-rule sums fall back to assigned reviewers")
}

func TestParse_NoteOnMergeRequest(t *testing.T) {
	body := `{
  "object_kind": "note",
  "user": {"id": 6, "username": "bob"},
  "project": {"id": 42, "full_path": "grp/app"},
  "object_attributes": {
    "id": 1244, "noteable_type": "MergeRequest", "action": "create",
    "system": false, "created_at": "2026-10-10T09:10:00.000Z"
  },
  "merge_request": {"id": 987654321, "iid": 12}
}`

	// act
	ev, err := Parse([]byte(body))

	// assert
	require.NoError(t, err)
	assert.True(t, ev.Processed())
	assert.Equal(t, "987654321", ev.MRRef)
	assert.True(t, ev.NoteCreate)
	assert.Equal(t, "bob", ev.Actor)
	assert.False(t, ev.When.IsZero())
}

func TestParse_NoteElsewhereIsInert(t *testing.T) {
	body := `{
  "object_kind": "note",
  "user": {"id": 6, "username": "bob"},
  "project": {"id": 42, "full_path": "grp/app"},
  "object_attributes": {"id": 1245, "noteable_type": "Issue", "action": "create"}
}`

	// act
	ev, err := Parse([]byte(body))

	// assert
	require.NoError(t, err)
	assert.Empty(t, ev.MRRef, "notes off merge requests name no thread or merge request")
}

func TestParse_PipelineLinkedAndUnlinked(t *testing.T) {
	linked := `{
  "object_kind": "pipeline",
  "user": {"id": 1, "username": "ci"},
  "project": {"id": 42, "full_path": "grp/app"},
  "object_attributes": {
    "id": 31, "status": "success",
    "created_at": "2026-10-10T09:15:00.000Z",
    "finished_at": "2026-10-10T09:20:00.000Z"
  },
  "merge_request": {"id": 987654321, "iid": 12}
}`

	// act
	ev, err := Parse([]byte(linked))

	// assert
	require.NoError(t, err)
	assert.Equal(t, "success", ev.PipelineStatus)
	assert.Equal(t, "987654321", ev.MRRef)
	assert.True(t, ev.When.Equal(time.Date(2026, 10, 10, 9, 20, 0, 0, time.UTC)),
		"finish time is the event time")

	// act: branch pipeline without linkage
	unlinked := `{"object_kind": "pipeline", "project": {"id": 42, "full_path": "grp/app"},
	  "object_attributes": {"id": 32, "status": "running", "ref": "main"}}`
	ev2, err := Parse([]byte(unlinked))

	// assert
	require.NoError(t, err)
	assert.Empty(t, ev2.MRRef)
}

func TestParse_Push(t *testing.T) {
	body := `{
  "object_kind": "push",
  "event_name": "push",
  "ref": "refs/heads/feature/auth",
  "project_id": 42,
  "project": {"id": 42, "full_path": "grp/app"},
  "user_name": "Alice", "user_username": "ada",
  "total_commits_count": 3,
  "commits": [
    {"timestamp": "2026-10-10T09:25:00.000Z"},
    {"timestamp": "2026-10-10T09:26:00.000Z"},
    {"timestamp": "2026-10-10T09:27:00.000Z"}
  ]
}`

	// act
	ev, err := Parse([]byte(body))

	// assert
	require.NoError(t, err)
	assert.Equal(t, "feature/auth", ev.PushBranch)
	assert.EqualValues(t, 42, ev.PushProjectID)
	assert.Equal(t, 3, ev.PushCommits)
	assert.Equal(t, "ada", ev.Actor)
	assert.True(t, ev.When.Equal(time.Date(2026, 10, 10, 9, 27, 0, 0, time.UTC)),
		"newest commit timestamps the push")
}

func TestParse_PushTagIsInert(t *testing.T) {
	// act
	ev, err := Parse([]byte(`{"object_kind":"push","ref":"refs/tags/v1","project":{"id":42,"full_path":"grp/app"},"total_commits_count":0}`))

	// assert
	require.NoError(t, err)
	assert.Empty(t, ev.PushBranch, "tag pushes attribute to nothing")
}

func TestParse_UnknownKindIsProcessedNo(t *testing.T) {
	// act
	ev, err := Parse([]byte(`{"object_kind":"wiki_page"}`))

	// assert
	require.NoError(t, err)
	assert.False(t, ev.Processed())
}
