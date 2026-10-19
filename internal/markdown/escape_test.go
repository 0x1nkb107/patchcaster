package markdown

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/v2/ast"
)

// hostileText is forge text that would add structure if it went into a
// template unescaped.
var hostileText = []string{
	"@channel [click](https://example.com)",
	"*bold* _em_ ~~gone~~ `code`",
	"# heading",
	"1. numbered",
	"- bullet",
	"> quote",
	"<b>html</b> &amp; &#x40;here",
	"https://example.com www.example.com ada@example.com",
	"| a | b |",
	"![image](https://example.com/i.png)",
	"line one\nline two\r\n\tline three",
	"trailing backslash \\",
	"\\*already escaped\\*",
	"\uE000here\uE001 @\uE000here\uE001",
}

// plainText returns the text of a document that holds one paragraph of
// text, and fails when the document holds any other node.
func plainText(t *testing.T, doc *Document) string {
	t.Helper()
	var text strings.Builder
	err := ast.Walk(doc.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Document, *ast.Paragraph:
		case *ast.Text:
			text.WriteString(node.Value.Value(doc.source))
		default:
			t.Errorf("forge text made a %s node", n.Kind())
		}
		return ast.WalkContinue, nil
	})
	require.NoError(t, err)
	return text.String()
}

func TestEscapedForgeTextParsesAsPlainText(t *testing.T) {
	for _, forgeText := range hostileText {
		t.Run(forgeText, func(t *testing.T) {
			doc := Parse(Escape(forgeText))

			want := strings.Join(strings.Fields(strings.NewReplacer("\uE000", "", "\uE001", "").Replace(forgeText)), " ")
			assert.Equal(t, want, plainText(t, doc))
		})
	}
}

func TestEscapeDropsControlCharacters(t *testing.T) {
	assert.Equal(t, "ab", Escape("a\x00\x1b\u009fb"))
}

func TestCodeShowsEscapedTextAsItIs(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"feature/auth", "`feature/auth`"},
		{"feature_x", "`feature_x`"},
		{"a`b", "``a`b``"},
		{"`start", "`` `start ``"},
		{"end``", "``` end`` ```"},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			code := Code(Escape(tc.text))
			assert.Equal(t, tc.want, code)

			if tc.text == "" {
				return
			}
			doc := Parse(code)
			span, ok := doc.root.FirstChild().FirstChild().(*ast.CodeSpan)
			require.True(t, ok, "Code must make a code span")
			assert.Equal(t, tc.text, span.Value.Value(doc.source))
		})
	}
}

func TestLinkTarget(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"https://gitlab.example/grp/app/-/merge_requests/12", "https://gitlab.example/grp/app/-/merge_requests/12"},
		{"http://gitlab.example/a", "http://gitlab.example/a"},
		{"HTTPS://gitlab.example/a", "https://gitlab.example/a"},
		{"https://gitlab.example/a b", "https://gitlab.example/a%20b"},
		{"https://gitlab.example/a(b)<c>", "https://gitlab.example/a%28b%29%3Cc%3E"},
		{"javascript:alert(1)", ""},
		{"data:text/html,x", ""},
		{"http:no-host", ""},
		{"/relative/path", ""},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			assert.Equal(t, tc.want, LinkTarget(tc.raw))
		})
	}
}
