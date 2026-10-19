package deliver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/message"
	"github.com/patchcaster-app/patchcaster/internal/plan"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// rootOf renders the built-in root of board.
func rootOf(r *Renderer, board *store.State) (string, error) {
	return r.Builtin(message.Root, message.Facts{Board: board})
}

// noticeOf renders the built-in notice that a plan names.
func noticeOf(r *Renderer, name string, board *store.State, ev *event.Event) (string, error) {
	kind, err := noticeKind(name)
	if err != nil {
		return "", err
	}
	return r.Builtin(kind, message.Facts{Board: board, Event: ev})
}

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
	"> by ada • grp/app • `feature/auth` ➜ `main` \\\n" +
	"> <https://gl.example/grp/app/-/merge_requests/12>"

func TestRenderer_RootQuietGolden(t *testing.T) {
	// act: no pipeline, no reviews, no comments, no edits — no status line
	// at all (SYSREQ-028 conditional segments).
	got, err := rootOf(mustRenderer(t), baseBoard())

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
	got, err := rootOf(mustRenderer(t), board)

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
	got, err := rootOf(mustRenderer(t), board)

	// assert
	require.NoError(t, err)
	assert.Contains(t, got,
		"**!12 Fix \\`bug\\` \\*urgent\\* \\[link\\](http:\u2060//evil) &lt;a href=x&gt;click&lt;/a&gt; \\~\u2060strike\\~\u2060 \\_under\\_**")
	assert.NotContains(t, got, "\n<a", "raw angle bracket may not reach the post")
	assert.NotContains(t, got, "@here \\\n>", "title may not forge an empty quoted line")
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

			got, err := noticeOf(mustRenderer(t), tc.notice, baseBoard(), ev)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRenderer_UnknownNoticeFails(t *testing.T) {
	// act
	_, err := noticeOf(mustRenderer(t), "does_not_exist", baseBoard(), &event.Event{MRRef: "1"})

	// assert: loud failure parks the delivery rather than posting nothing.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no notice template")
}

func TestRenderer_NoticeEscapesActor(t *testing.T) {
	// act
	got, err := noticeOf(mustRenderer(t), plan.Comment, baseBoard(),
		&event.Event{MRRef: "1", Actor: "@here *sponsored*"})

	// assert: a user-controlled name may not ping the channel (SYSREQ-003).
	require.NoError(t, err)
	assert.Equal(t, "💬 @\u2060here \\*sponsored\\* commented", got)
}
