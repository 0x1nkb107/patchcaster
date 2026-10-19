package message

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltinsCoverEveryKindAndPassTheCheck(t *testing.T) {
	builtins, err := Builtins()
	require.NoError(t, err)

	for _, kind := range Kinds {
		t.Run(string(kind), func(t *testing.T) {
			template, ok := builtins[kind]
			require.True(t, ok, "no built-in template")
			assert.Equal(t, kind, template.Kind())
			assert.Equal(t, Version1, template.Version())
			assert.NoError(t, Check(template))
		})
	}
}

func TestCheckRefusesATemplateThatFailsOnASample(t *testing.T) {
	template, err := Parse(Root, Version1, "ok\n{{if eq .Status.Pipeline \"failed\"}}{{.Nope}}{{end}}")
	require.NoError(t, err)

	requireRefusal(t, Check(template), 2, "with a pipeline failed: ")
}

func TestCheckRefusesANoticeThatFailsWithACommitCount(t *testing.T) {
	template, err := Parse(Pushed, Version1, "{{with .Event.Commits}}{{.Nope}}{{end}}")
	require.NoError(t, err)

	requireRefusal(t, Check(template), 1, "and a commit count: ")
}

func TestCheckRefusesAMessageLongerThanMattermostAccepts(t *testing.T) {
	template, err := Parse(Root, Version1, strings.Repeat("{{.MergeRequest.Title}}", 13))
	require.NoError(t, err)

	requireRefusal(t, Check(template), 0, "with the longest values the message holds 16575 characters, more than the 16383 that Mattermost accepts")
}

func TestCheckAcceptsAMessageJustShortEnough(t *testing.T) {
	template, err := Parse(Root, Version1, strings.Repeat("{{.MergeRequest.Title}}", 12))
	require.NoError(t, err)

	assert.NoError(t, Check(template))
}

func TestPreviewShowsTheExamplesButNotTheSizeCheck(t *testing.T) {
	template, err := Parse(Closed, Version1, "{{.Event.Actor}} closed !{{.MergeRequest.Number}}{{with .Event.Commits}} after {{.}} commits{{end}}")
	require.NoError(t, err)

	renderings, err := Preview(template)

	require.NoError(t, err)
	require.NotEmpty(t, renderings)
	assert.Equal(t, Rendering{Description: "an open merge request", Markdown: "bob closed !12"}, renderings[0])
	assert.Equal(t, Rendering{Description: "an open merge request and a commit count", Markdown: "bob closed !12 after 3 commits"}, renderings[1])
	for _, rendering := range renderings {
		assert.NotContains(t, rendering.Description, "longest")
	}
}

func TestPreviewRefusesWhatCheckRefuses(t *testing.T) {
	template, err := Parse(Root, Version1, strings.Repeat("{{.MergeRequest.Title}}", 13))
	require.NoError(t, err)

	_, err = Preview(template)

	requireRefusal(t, err, 0, "more than the 16383 that Mattermost accepts")
}
