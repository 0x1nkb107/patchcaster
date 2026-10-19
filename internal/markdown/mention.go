package markdown

import (
	"bytes"
	"unicode/utf8"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
)

// A mention's markup is "@" with its target between these two characters of
// the Private Use Area. Escape removes them from forge text, so only the
// templates' mention function writes a mention (ADR-037).
const (
	mentionOpen  = '\uE000'
	mentionClose = '\uE001'
)

// MentionTarget names who a mention pings.
type MentionTarget string

// MentionHere pings the people in the channel who are online.
const MentionHere MentionTarget = "here"

// mentionTargets lists the targets a mention may name.
var mentionTargets = map[MentionTarget]bool{
	MentionHere: true,
}

// MentionMarkup returns the markup that the parser reads as a mention of
// target, and false when no mention can name target.
func MentionMarkup(target MentionTarget) (string, bool) {
	if !mentionTargets[target] {
		return "", false
	}
	return "@" + string(mentionOpen) + string(target) + string(mentionClose), true
}

// Mention is a node that pings people on the sink's platform.
type Mention struct {
	ast.BaseInline

	Target MentionTarget
}

// KindMention is the NodeKind of a Mention.
var KindMention = ast.NewNodeKind("Mention")

// Kind implements ast.Node.
func (n *Mention) Kind() ast.NodeKind {
	return KindMention
}

// Dump implements ast.Node.
func (n *Mention) Dump(_ []byte) *ast.NodeDump {
	return ast.NewNodeDump(n, map[string]any{"Target": string(n.Target)})
}

// newMention returns a Mention of target.
func newMention(target MentionTarget) *Mention {
	n := &Mention{Target: target}
	n.Init(n)
	return n
}

// mentionParser reads the markup of MentionMarkup. Any other "@" stays text.
type mentionParser struct{}

// Trigger implements parser.InlineParser.
func (mentionParser) Trigger() []byte {
	return []byte{'@'}
}

// Parse implements parser.InlineParser.
func (mentionParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	open := utf8.AppendRune(nil, mentionOpen)
	closing := utf8.AppendRune(nil, mentionClose)

	afterAt := line[1:]
	if !bytes.HasPrefix(afterAt, open) {
		return nil
	}
	length := bytes.Index(afterAt, closing)
	if length < 0 {
		return nil
	}
	target := MentionTarget(afterAt[len(open):length])
	if !mentionTargets[target] {
		return nil
	}
	block.Advance(1 + length + len(closing))
	return newMention(target)
}
