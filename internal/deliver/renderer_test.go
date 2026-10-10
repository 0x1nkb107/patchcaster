package deliver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0x5a17ed/patchcaster/internal/event"
	"github.com/0x5a17ed/patchcaster/internal/plan"
	"github.com/0x5a17ed/patchcaster/internal/store"
)

func mustRenderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer()
	require.NoError(t, err)
	return r
}

func baseBoard() *store.State {
	return &store.State{
		MrRef:        "987654321",
		IID:          12,
		Title:        "Add flux capacitor",
		Author:       "ada",
		ProjectPath:  "grp/app",
		SourceBranch: "feature/auth",
		TargetBranch: "main",
		URL:          "https://gl.example/grp/app/-/merge_requests/12",
		Open:         true,
	}
}

const wantQuietRoot = "> @here :bell: \\\n" +
	"> **!12 Add flux capacitor** \\\n" +
	"> by @ada • grp/app • `feature/auth` ➜ `main` \\\n" +
	"> https://gl.example/grp/app/-/merge_requests/12"

func TestRenderer_RootQuietGolden(t *testing.T) {
	// act: no pipeline, no reviews, no comments, no edits — no status line
	// at all (SYSREQ-028 conditional segments).
	got, err := mustRenderer(t).Root(baseBoard())

	// assert
	require.NoError(t, err)
	assert.Equal(t, wantQuietRoot, got)
}

func TestRenderer_RootFullStatusGolden(t *testing.T) {
	// arrange: every signal present, in the owner's example shape.
	board := baseBoard()
	board.PipelineStatus = "running"
	board.ReviewersDone, board.ReviewersRequired = 1, 2
	board.Comments = 3
	board.Edits = 1

	// act
	got, err := mustRenderer(t).Root(board)

	// assert
	require.NoError(t, err)
	assert.Equal(t, wantQuietRoot+" \\\n> 🟡 Pipeline running • 👀 1/2 reviewers • 💬 3 comments • 📝 1 edit", got)
}

func TestRenderer_RootNeutralizesAdversarialTitle(t *testing.T) {
	// arrange: Markdown structural characters, injected angle brackets, and a
	// newline that would fake a second line (SYSREQ-003 attack surface).
	board := baseBoard()
	board.Title = "Fix `bug` *urgent* [link](http://evil)\n<a href=x>click</a> ~strike~ _under_"

	// act
	got, err := mustRenderer(t).Root(board)

	// assert
	require.NoError(t, err)
	assert.Contains(t, got,
		"**!12 Fix 'bug' \\*urgent\\* \\[link\\](http://evil) &lt;a href=x&gt;click&lt;/a&gt; \\~strike\\~ \\_under\\_**")
	assert.NotContains(t, got, "\n<a", "raw angle bracket may not reach the post")
	assert.NotContains(t, got, "@here \\\n>", "title may not forge an empty quoted line")
}

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
		{"unknown status degrades honestly", func(b *store.State) { b.PipelineStatus = "quantum*leap" }, "⚪ Pipeline quantum\\*leap"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			board := baseBoard()
			tc.mutate(board)
			assert.Equal(t, tc.want, StatusLine(board))
		})
	}
}

func TestRenderer_NoticeGolden(t *testing.T) {
	noticeEvent := func() *event.Event {
		return &event.Event{MRRef: "987", IID: 12, Actor: "bob"}
	}
	three := 3

	tests := []struct {
		notice string
		edit   func(*event.Event)
		want   string
	}{
		{plan.Comment, nil, "💬 bob commented"},
		{plan.PipelinePass, nil, "🟢 Pipeline passed"},
		{plan.PipelineFail, nil, "🔴 Pipeline failed"},
		{plan.Reviewed, nil, "👀 bob reviewed the MR"},
		{plan.Changes, nil, "🚨 bob asks for changes!"},
		{plan.Approval, nil, "✅ Review was approved by bob!"},
		{plan.Push, func(ev *event.Event) { ev.PushCount = &three }, "📝 bob pushed 3 new commits"},
		{plan.Push, nil, "📝 bob pushed new commits"},
		{plan.Merged, nil, "🎉 bob merged !12"},
		{plan.Closed, nil, "🔒 bob closed !12"},
	}
	for _, tc := range tests {
		t.Run(tc.notice+"/"+tc.want, func(t *testing.T) {
			ev := noticeEvent()
			if tc.edit != nil {
				tc.edit(ev)
			}

			got, err := mustRenderer(t).Notice(tc.notice, ev)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRenderer_UnknownNoticeFails(t *testing.T) {
	// act
	_, err := mustRenderer(t).Notice("does_not_exist", &event.Event{MRRef: "1"})

	// assert: loud failure parks the delivery rather than posting nothing.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no notice template")
}

func TestRenderer_NoticeEscapesActor(t *testing.T) {
	// act
	got, err := mustRenderer(t).Notice(plan.Comment,
		&event.Event{MRRef: "1", Actor: "@here *sponsored*"})

	// assert: a user-controlled name may not ping the channel (SYSREQ-003).
	require.NoError(t, err)
	assert.Equal(t, `💬 \@here \*sponsored\* commented`, got)
}
