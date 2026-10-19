package deliver

import (
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/plan"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// These tests read a message the way Mattermost does, with a CommonMark
// parser that links bare addresses, as an independent check on what the
// renderer posts (SYSREQ-051).

// parsedMessage is a posted message as a Markdown parser reads it.
type parsedMessage struct {
	source []byte
	root   ast.Node
}

// readMessage parses a posted message.
func readMessage(message string) parsedMessage {
	reader := parser.New(parser.WithExtensions(
		extension.NewTableParser(),
		extension.NewStrikethroughParser(),
		extension.NewLinkifyParser(),
	))
	source := []byte(message)
	return parsedMessage{source: source, root: reader.Parse(source)}
}

// shape lists the kinds of every node but text, in document order. Two
// messages of the same shape differ in their text only.
func (m parsedMessage) shape(t *testing.T) []string {
	t.Helper()
	var kinds []string
	err := ast.Walk(m.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if _, isText := n.(*ast.Text); entering && !isText {
			kinds = append(kinds, n.Kind().String())
		}
		return ast.WalkContinue, nil
	})
	require.NoError(t, err)
	return kinds
}

// text returns what the text nodes under n show, without word joiners.
func (m parsedMessage) text(n ast.Node) string {
	var shown strings.Builder
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if text, isText := child.(*ast.Text); isText {
			shown.WriteString(text.Value.Value(m.source))
		}
	}
	return strings.ReplaceAll(shown.String(), "\u2060", "")
}

// first returns the first node of kind, or nil.
func (m parsedMessage) first(kind ast.NodeKind) ast.Node {
	var found ast.Node
	_ = ast.Walk(m.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == kind {
			found = n
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

// inMentionWord reports whether Mattermost keeps c inside a word when it
// looks for mentions. Every other character ends the word.
func inMentionWord(c rune) bool {
	return c == ':' || c == '.' || c == '-' || c == '_' || c == '@' || unicode.IsLetter(c) || unicode.IsNumber(c)
}

// mattermostMentions returns the words that Mattermost's server would read
// as mentions in a message. Like getExplicitMentions in
// server/channels/app/notification.go of mattermost/mattermost, it reads
// runs of text nodes only, and it splits them into words as ProcessText in
// server/channels/app/mention_parser_standard.go does.
func mattermostMentions(message string) []string {
	parsed := readMessage(message)
	var runs []string
	var run strings.Builder
	_ = ast.Walk(parsed.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		text, isText := n.(*ast.Text)
		if !isText {
			runs = append(runs, run.String())
			run.Reset()
			return ast.WalkContinue, nil
		}
		run.WriteString(text.Value.Value(parsed.source))
		return ast.WalkContinue, nil
	})
	runs = append(runs, run.String())

	var mentions []string
	for _, text := range runs {
		for _, word := range strings.FieldsFunc(text, func(c rune) bool { return !inMentionWord(c) }) {
			word = strings.TrimLeft(word, ":.-_")
			if len(word) > 1 && word[0] == '@' {
				mentions = append(mentions, word)
			}
		}
	}
	return mentions
}

// shownAs returns forge text as a message shows it: white space collapsed
// and trimmed, control characters and mention markers gone.
func shownAs(forgeText string) string {
	kept := strings.Map(func(c rune) rune {
		switch {
		case unicode.IsSpace(c):
			return ' '
		case unicode.IsControl(c), c == '\uE000', c == '\uE001':
			return -1
		default:
			return c
		}
	}, forgeText)
	return strings.Join(strings.Fields(kept), " ")
}

// boardOf returns a board whose every forge value is text.
func boardOf(text string) *store.State {
	board := baseBoard()
	board.Title = text
	board.Author = text
	board.ProjectPath = text
	board.SourceBranch = text
	board.TargetBranch = text
	board.PipelineStatus = text
	return board
}

func TestRendererLetsNoForgeTextMention(t *testing.T) {
	attempts := []string{"@here", "@channel", "@ALL", "@ada", "hi @here!", "\\@here", "(@channel)"}
	for _, attempt := range attempts {
		t.Run(attempt, func(t *testing.T) {
			root, rootErr := mustRenderer(t).Root(boardOf(attempt))
			notice, noticeErr := mustRenderer(t).Notice(plan.Comment, boardOf(attempt), &event.Event{MRRef: "1", Actor: attempt})

			require.NoError(t, rootErr)
			require.NoError(t, noticeErr)
			// The root's own header is the template's only mention.
			assert.Equal(t, []string{"@here"}, mattermostMentions(root))
			assert.Empty(t, mattermostMentions(notice))
		})
	}
}

// FuzzForgeTextAddsNoStructure feeds hostile text into every forge value of
// the built-in templates (SYSREQ-051). The message keeps the shape it has
// for harmless text, the title shows as it is, and nothing but the root's
// header mentions anyone.
func FuzzForgeTextAddsNoStructure(f *testing.F) {
	seeds := []string{
		"@channel [click](https://example.com)",
		"*bold* _em_ ~~gone~~ `code` ``double``",
		"# heading", "1. numbered", "- bullet", "> quote", "| a | b |",
		"<b>html</b> &amp; &#x40;here <https://example.com>",
		"https://example.com www.example.com ada@example.com ~town-square",
		"![image](https://example.com/i.png) [ref]: https://example.com",
		"line one\nline two\r\n\tline three", "trailing backslash \\",
		"\\*already escaped\\*", "\uE000here\uE001 @\uE000here\uE001",
		"**", "`", "a`b", "!12 **x** y", "\u2060@\u2060here",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	renderer, err := NewRenderer()
	require.NoError(f, err)
	quietRoot, err := renderer.Root(boardOf("x"))
	require.NoError(f, err)
	quietNotice, err := renderer.Notice(plan.Comment, boardOf("x"), &event.Event{MRRef: "1", Actor: "x"})
	require.NoError(f, err)

	f.Fuzz(func(t *testing.T, forgeText string) {
		if shownAs(forgeText) == "" {
			t.Skip("the templates expect forge values that show something")
		}
		root, err := renderer.Root(boardOf(forgeText))
		require.NoError(t, err)
		notice, err := renderer.Notice(plan.Comment, boardOf(forgeText), &event.Event{MRRef: "1", Actor: forgeText})
		require.NoError(t, err)

		parsedRoot := readMessage(root)
		assert.Equal(t, readMessage(quietRoot).shape(t), parsedRoot.shape(t), "root:\n%s", root)
		assert.Equal(t, readMessage(quietNotice).shape(t), readMessage(notice).shape(t), "notice:\n%s", notice)
		title := strings.ReplaceAll(shownAs(forgeText), "\u2060", "")
		assert.Equal(t, "!12 "+title, parsedRoot.text(parsedRoot.first(ast.KindStrong)), "root:\n%s", root)
		assert.Equal(t, []string{"@here"}, mattermostMentions(root), "root:\n%s", root)
		assert.Empty(t, mattermostMentions(notice), "notice:\n%s", notice)
	})
}
