package deliver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0x5a17ed/patchcaster/internal/event"
)

func mustRenderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer()
	require.NoError(t, err)
	return r
}

func baseEvent() *event.Event {
	return &event.Event{
		Action:      "open",
		MRRef:       "987654321",
		IID:         12,
		ProjectPath: "grp/app",
		Title:       "Add flux capacitor",
		Author:      "ada",
		Actor:       "ada",
		URL:         "https://gl.example/grp/app/-/merge_requests/12",
		When:        time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC),
	}
}

func TestRenderer_RootGolden(t *testing.T) {
	// act
	got, err := mustRenderer(t).Root(baseEvent())

	// assert
	require.NoError(t, err)
	assert.Equal(t,
		"**Add flux capacitor** (grp/app!12) opened by @ada\n"+
			"https://gl.example/grp/app/-/merge_requests/12",
		got)
}

func TestRenderer_ReplyGolden(t *testing.T) {
	// arrange
	ev := baseEvent()
	ev.Action = "approved"
	ev.Actor = "grace"

	// act
	got, err := mustRenderer(t).Reply(ev)

	// assert
	require.NoError(t, err)
	assert.Equal(t,
		"approved — **Add flux capacitor** (grp/app!12) by @grace\n"+
			"https://gl.example/grp/app/-/merge_requests/12",
		got)
}

func TestRenderer_NeutralizesAdversarialTitle(t *testing.T) {
	// arrange: Markdown structural characters, injected angle brackets, and a
	// newline that would fake a second line (SYSREQ-003 attack surface).
	ev := baseEvent()
	ev.Title = "Fix `bug` *urgent* [link](http://evil)\n<a href=x>click</a> ~strike~ _under_"

	// act
	got, err := mustRenderer(t).Root(ev)

	// assert
	require.NoError(t, err)
	assert.Equal(t,
		"**Fix 'bug' \\*urgent\\* \\[link\\](http://evil) &lt;a href=x&gt;click&lt;/a&gt; \\~strike\\~ \\_under\\_** (grp/app!12) opened by @ada\n"+
			"https://gl.example/grp/app/-/merge_requests/12",
		got)
	assert.NotContains(t, got, "\n<a", "raw angle bracket may not reach the post")
}

func TestRenderer_SanitizesURL(t *testing.T) {
	// arrange: newline and angle brackets in a URL must not survive.
	ev := baseEvent()
	ev.URL = "https://x.example/a\n<b>c</b>"

	// act
	got, err := mustRenderer(t).Reply(ev)

	// assert
	require.NoError(t, err)
	assert.Contains(t, got, "https://x.example/abc")
	assert.NotContains(t, got, "\n<b>")
}

func TestRenderer_OnlySeesEscapedView(t *testing.T) {
	// arrange: the view is the template's whole universe.
	ev := baseEvent()
	ev.Title = "**bold**"
	view := NewView(ev)

	// act & assert
	assert.NotContains(t, view.Title, "**bold**", "raw markdown must not survive construction")
}
