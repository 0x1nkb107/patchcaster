package markdown

import (
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/util"
)

// Document is a parsed message, ready for a platform's renderer.
type Document struct {
	source []byte
	root   ast.Node
}

// mentionPriority runs the mention parser before goldmark's own parsers, so
// a mention's markup never reaches them.
const mentionPriority = 50

// newParser reads CommonMark with the GitHub Flavored Markdown extensions
// for tables, strikethrough and autolinks (ADR-037), plus mentions. Bare
// addresses become links only for http and https.
func newParser() parser.Parser {
	return parser.New(
		parser.WithExtensions(
			extension.NewTableParser(),
			extension.NewStrikethroughParser(),
			extension.NewLinkifyParser(extension.WithAllowedProtocols([]string{"http:", "https:"})),
		),
		parser.WithInlineParsers(util.Prioritized[parser.InlineParser](mentionParser{}, mentionPriority)),
	)
}

// Parse reads the Markdown a template rendered.
func Parse(markdown string) *Document {
	source := []byte(markdown)
	return &Document{source: source, root: newParser().Parse(source)}
}
