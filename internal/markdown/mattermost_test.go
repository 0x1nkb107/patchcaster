package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// here is the markup of a mention of the people online in the channel.
func here(t *testing.T) string {
	t.Helper()
	markup, ok := MentionMarkup(MentionHere)
	require.True(t, ok)
	return markup
}

func TestMattermostRendersEachConstruct(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     string
	}{
		{"paragraphs", "one\n\ntwo", "one\n\ntwo"},
		{"soft line break", "one\ntwo", "one\ntwo"},
		{"hard line break", "one \\\ntwo", "one \\\ntwo"},
		{"emphasis", "*a* _b_ **c** __d__ ~~e~~", "*a* *b* **c** **d** ~~e~~"},
		{"code span", "run ``a`b`` now", "run ``a`b`` now"},
		{"code span keeps its spaces", "`a  b`", "`a  b`"},
		{"heading", "## Title", "## Title"},
		{"setext heading", "Title\n=====", "# Title"},
		{"thematic break", "a\n\n***\n\nb", "a\n\n---\n\nb"},
		{"blockquote", "> a \\\n> b\n>\n> c", "> a \\\n> b\n>\n> c"},
		{"bullet list", "* a\n* b", "- a\n- b"},
		{"numbered list", "3) a\n4) b", "3. a\n4. b"},
		{"loose list", "- a\n\n- b", "- a\n\n- b"},
		{"nested list", "- a\n  - b\n- c", "- a\n  - b\n- c"},
		{"list item with a second line", "1. a \\\n   b", "1. a \\\n   b"},
		{"empty list item", "-\n- b", "-\n- b"},
		{"fenced code", "```go\nx := `a`\n```", "```go\nx := `a`\n```"},
		{"code holding a fence", "````\n```\n````", "````\n```\n````"},
		{"indented code", "    x := 1", "```\nx := 1\n```"},
		{"odd code language", "```go;rm\nx\n```", "```\nx\n```"},
		{"table", "| a | b | c |\n|:--|:-:|--:|\n| 1 | 2 | 3 |", "| a | b | c |\n| :--- | :---: | ---: |\n| 1 | 2 | 3 |"},
		{"link", "[site](https://example.com/x)", "[site](https://example.com/x)"},
		{"link with a title", "[site](https://example.com/x \"Example\")", "[site](https://example.com/x)"},
		{"mail link", "[mail](mailto:ada@example.com)", "[mail](mailto:ada@example.com)"},
		{"script link", "[site](javascript:alert(1))", "site"},
		{"relative link", "[site](/x)", "site"},
		{"autolink", "<https://example.com/x>", "<https://example.com/x>"},
		{"bare address", "see https://example.com/x", "see <https://example.com/x>"},
		{"bare web host", "see www.example.com", "see <http://www.example.com>"},
		{"email autolink", "<ada@example.com>", "<ada@example.com>"},
		{"other scheme autolink", "<ftp://example.com/x>", "ftp:\u2060//example.com/x"},
		{"image", "![logo](https://example.com/l.png)", "[logo](https://example.com/l.png)"},
		{"inline html", "a <b>bold</b>", "a &lt;b&gt;bold&lt;/b&gt;"},
		{"html block", "<div>\nx\n</div>", "&lt;div&gt;\nx\n&lt;/div&gt;"},
		{"escaped markup", "\\*a\\* \\_b\\_ \\[c\\] \\`d\\` \\| e \\\\", "\\*a\\* \\_b\\_ \\[c\\] \\`d\\` \\| e \\\\"},
		{"heading marker as text", "\\# a", "\\# a"},
		{"list markers as text", "\\- a", "\\- a"},
		{"number as text", "1\\. a", "1\\. a"},
		{"entities", "a &amp; b &lt;c&gt;", "a &amp; b &lt;c&gt;"},
		{"mention as text", "@here and @ada", "@\u2060here and @\u2060ada"},
		{"channel name as text", "~town-square", "\\~\u2060town-square"},
		{"tildes as text", "\\~\\~gone\\~\\~", "\\~\u2060\\~\u2060gone\\~\u2060\\~\u2060"},
		{"address as text", "https\\://example\\.com and WWW\\.example\\.com", "https:\u2060//example.com and WWW\u2060.example.com"},
		{"word ending in www", "awww\\.example", "awww.example"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Mattermost(Parse(tc.markdown)))
		})
	}
}

func TestMattermostRendersMentions(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     string
	}{
		{"alone", here(t), "@here"},
		{"before a space", here(t) + " :bell:", "@here :bell:"},
		{"before a letter", here(t) + "x", "@here\u2060x"},
		{"before an emoji code", here(t) + ":bell:", "@here\u2060:bell:"},
		{"before a full stop", here(t) + ".", "@here\u2060."},
		{"in a quote", "> " + here(t) + " \\\n> b", "> @here \\\n> b"},
		{"inside a code span", "`" + here(t) + "`", "`@here`"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Mattermost(Parse(tc.markdown)))
		})
	}
}

func TestMentionMarkupNamesKnownTargetsOnly(t *testing.T) {
	_, ok := MentionMarkup("channel")
	assert.False(t, ok)
}

func TestUnknownMentionMarkupStaysText(t *testing.T) {
	got := Mattermost(Parse("@\uE000channel\uE001 and @\uE000here"))

	assert.Equal(t, "@\u2060channel and @\u2060here", got)
}
