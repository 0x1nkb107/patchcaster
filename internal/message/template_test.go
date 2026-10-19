package message

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireRefusal asserts that err is an Error at line whose cause contains
// cause.
func requireRefusal(t *testing.T, err error, line int, cause string) {
	t.Helper()
	var refusal *Error
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, line, refusal.Line, "line of %q", refusal.Cause)
	assert.Contains(t, refusal.Cause, cause)
}

func TestParseRefusesWithLineAndCause(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		line  int
		cause string
	}{
		{"unclosed action", "ok\n{{.MergeRequest.Title", 2, "unclosed action"},
		{"unknown function", "{{shout .MergeRequest.Title}}", 1, `function "shout" not defined`},
		{"printf", "ok\n\n{{printf \"%s\" .MergeRequest.Title}}", 3, "printf is not a template function"},
		{"slice", "{{slice .MergeRequest.Title 1}}", 1, "slice is not a template function"},
		{"index", "{{index .MergeRequest.Title 1}}", 1, "index is not a template function"},
		{"len", "{{len .MergeRequest.Title}}", 1, "len is not a template function"},
		{"call", "{{call .MergeRequest.Title}}", 1, "call is not a template function"},
		{"html", "{{html .MergeRequest.Title}}", 1, "html is not a template function"},
		{"function in a condition", "{{if print .MergeRequest.Open}}x{{end}}", 1, "print is not a template function"},
		{"function in an else branch", "{{if .MergeRequest.Open}}x{{else}}\n{{urlquery .MergeRequest.URL}}{{end}}", 2, "urlquery is not a template function"},
		{"function in parentheses", "{{code (js .MergeRequest.Title)}}", 1, "js is not a template function"},
		{"range", "{{range 3}}x{{end}}", 1, "range is not available"},
		{"template", "{{template \"x\"}}", 1, "template is not available"},
		{"define", "{{define \"x\"}}y{{end}}", 0, "define and block are not available"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(Root, Version1, tc.text)

			requireRefusal(t, err, tc.line, tc.cause)
		})
	}
}

func TestParseRefusesAnUnknownVersion(t *testing.T) {
	_, err := Parse(Root, Version(2), "x")

	requireRefusal(t, err, 0, "data model version 2 does not exist")
}

func TestParseAcceptsTheDocumentedFunctions(t *testing.T) {
	text := `{{mention "channel"}} {{code .MergeRequest.SourceBranch}} {{plural .Status.Comments "comment" "comments"}}` +
		`{{if and (not .MergeRequest.Open) (or (eq .Status.Pipeline "failed") (ne .Status.Edits 0))}}!{{end}}` +
		`{{if and (lt .Status.ReviewersDone .Status.ReviewersRequired) (le 1 2) (gt 2 1) (ge 2 2)}}?{{end}}` +
		`{{$title := .MergeRequest.Title}}{{with $title}}{{.}}{{end}}`

	_, err := Parse(Root, Version1, text)

	require.NoError(t, err)
}

func TestRenderNamesTheLineOfAFailure(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		line  int
		cause string
	}{
		{"unknown field", "ok\n{{.MergeRequest.Nope}}", 2, "can't evaluate field Nope"},
		{"unknown mention", "{{mention \"everyone\"}}", 1, "mention knows here and channel"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			template, err := Parse(Root, Version1, tc.text)
			require.NoError(t, err)

			_, err = template.Render(Facts{Board: sampleBoard()})

			requireRefusal(t, err, tc.line, tc.cause)
		})
	}
}

func TestRenderWritesMarkdown(t *testing.T) {
	template, err := Parse(Root, Version1, `{{mention "here"}} **{{.MergeRequest.Title}}** {{code .MergeRequest.SourceBranch}} <{{.MergeRequest.URL}}>`)
	require.NoError(t, err)
	board := sampleBoard()
	board.Title = "a_b"

	rendered, err := template.Render(Facts{Board: board})

	require.NoError(t, err)
	assert.Equal(t, "@\uE000here\uE001 **a\\_b** `feature/flux` <https://gitlab.example/grp/app/-/merge_requests/12>", rendered)
	assert.False(t, strings.HasSuffix(rendered, "\n"))
}
