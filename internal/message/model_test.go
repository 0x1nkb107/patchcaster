package message

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

func TestStatusLineSegmentPresence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*store.State)
		want   string
	}{
		{"empty board renders no line", func(*store.State) {}, ""},
		{"pipeline linked", func(b *store.State) { b.PipelineStatus = "success" }, "🟢 Pipeline passed"},
		{"pipeline waiting", func(b *store.State) { b.PipelineStatus = "waiting_for_resource" }, "⚪ Pipeline waiting"},
		{"done without a required denominator stays hidden", func(b *store.State) { b.ReviewersDone = 5 }, ""},
		{"reviews", func(b *store.State) { b.ReviewersDone, b.ReviewersRequired = 2, 2 }, "👀 2/2 reviewers"},
		{"zero counters stay hidden", func(b *store.State) { b.Comments, b.Edits = 0, 0 }, ""},
		{"counts appear when real", func(b *store.State) { b.Comments, b.Edits = 1, 2 }, "💬 1 comment • 📝 2 edits"},
		{"unknown status degrades honestly", func(b *store.State) { b.PipelineStatus = "quantum*leap" }, "⚪ Pipeline quantum*leap"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			board := sampleBoard()
			tc.mutate(board)
			assert.Equal(t, tc.want, statusLine(board))
		})
	}
}

func TestPipelineStateIsForgeNeutral(t *testing.T) {
	tests := map[string]string{
		"":                     "",
		"running":              "running",
		"success":              "passed",
		"failed":               "failed",
		"created":              "waiting",
		"pending":              "waiting",
		"waiting_for_resource": "waiting",
		"manual":               "waiting",
		"canceled":             "canceled",
		"skipped":              "skipped",
		"quantum":              "unknown",
	}
	for status, want := range tests {
		t.Run(status, func(t *testing.T) {
			assert.Equal(t, want, pipelineState(status))
		})
	}
}

func TestDataV1EscapesForgeText(t *testing.T) {
	board := sampleBoard()
	board.Title = "*urgent* fix"
	board.URL = "javascript:alert(1)"
	board.PipelineStatus = "success"
	board.Comments = 2

	data := dataV1(Root, Facts{Board: board})

	assert.Equal(t, Text("\\*urgent\\* fix"), data.MergeRequest.Title)
	assert.Equal(t, Text("feature\\/flux"), data.MergeRequest.SourceBranch)
	assert.Empty(t, data.MergeRequest.URL, "only http and https addresses become link targets")
	assert.Equal(t, "passed", data.Status.Pipeline)
	assert.Equal(t, Text("🟢 Pipeline passed • 💬 2 comments"), data.Status.Line)
	assert.Equal(t, Event{}, data.Event, "the root has no event")
}

func TestDataV1GivesNoticesTheirEvent(t *testing.T) {
	commits := 3
	ev := &event.Event{Actor: "@bob", PushCount: &commits}

	data := dataV1(Pushed, Facts{Board: sampleBoard(), Event: ev})

	assert.Equal(t, Event{Kind: Pushed, Actor: "\\@bob", Commits: 3}, data.Event)
}
