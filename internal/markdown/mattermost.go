package markdown

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/v2/ast"
	east "github.com/yuin/goldmark/v2/extension/ast"
)

// wordJoiner is invisible, and Mattermost ends a word at it. In plain text
// it follows an "@" or a "~", and it splits "://" and "www.", so Mattermost
// reads no mention, channel link or address there (SYSREQ-051). After a
// mention it keeps the mention's word whole. See ProcessText in
// server/channels/app/mention_parser_standard.go of mattermost/mattermost.
const wordJoiner = "\u2060"

// inMentionWord reports whether Mattermost keeps c inside a word when it
// looks for mentions.
func inMentionWord(c rune) bool {
	return c == ':' || c == '.' || c == '-' || c == '_' || c == '@' || unicode.IsLetter(c) || unicode.IsNumber(c)
}

// withoutMentionMarkup removes the markup of mentions from code, where the
// parser takes no mention.
func withoutMentionMarkup(code string) string {
	return strings.NewReplacer(string(mentionOpen), "", string(mentionClose), "").Replace(code)
}

// MattermostMaxRunes is the longest message, in characters, that every
// Mattermost server in use accepts: the limit of a server whose message
// column holds 65,535 bytes, four bytes per character. Current servers
// accept more (determineMaxPostSize in
// server/channels/store/sqlstore/post_store.go of mattermost/mattermost).
const MattermostMaxRunes = 16383

// Mattermost renders doc as the Markdown that Mattermost shows (ADR-037).
// Text shows as it is, so only the document's own nodes add structure. A
// mention node is the only thing that pings, and only links and autolinks
// link. Mattermost shows every construct of the common dialect, apart from
// two that degrade: an image becomes a link to it, and raw HTML shows as
// text.
func Mattermost(doc *Document) string {
	r := mattermostRenderer{source: doc.source}
	return r.blocks(doc.root)
}

// mattermostRenderer holds the source that the document's values point into.
type mattermostRenderer struct {
	source []byte
}

// blocks renders the block children of parent, separated by blank lines.
func (r mattermostRenderer) blocks(parent ast.Node) string {
	return r.joinBlocks(parent, "\n\n")
}

// joinBlocks renders the block children of parent, separated by sep.
func (r mattermostRenderer) joinBlocks(parent ast.Node, sep string) string {
	var rendered []string
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		if block := r.block(child); block != "" {
			rendered = append(rendered, block)
		}
	}
	return strings.Join(rendered, sep)
}

// block renders one block node.
func (r mattermostRenderer) block(n ast.Node) string {
	switch node := n.(type) {
	case *ast.Paragraph:
		return r.inlines(node)
	case *ast.Heading:
		return strings.Repeat("#", node.Level) + " " + r.inlinesFrom(node, false)
	case *ast.ThematicBreak:
		return "---"
	case *ast.CodeBlock:
		return r.codeBlock(node)
	case *ast.Blockquote:
		return prefixLines(r.blocks(node), "> ", ">")
	case *ast.List:
		return r.list(node)
	case *ast.HTMLBlock:
		return r.htmlBlock(node)
	case *east.Table:
		return r.table(node)
	case *ast.LinkReferenceDefinition:
		// The parser resolved every reference already.
		return ""
	default:
		return r.blocks(node)
	}
}

// prefixLines puts prefix before each line of text, and emptyPrefix before
// each empty line.
func prefixLines(text, prefix, emptyPrefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = emptyPrefix
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// codeBlock renders a fenced code block whose fence is longer than any run
// of backticks inside it. Only a plain language name survives as its info.
func (r mattermostRenderer) codeBlock(node *ast.CodeBlock) string {
	content := withoutMentionMarkup(strings.TrimSuffix(node.Value.Str(r.source), "\n"))
	fence := strings.Repeat("`", max(3, longestRun(content, '`')+1))
	language, _ := node.Language(r.source)
	if !isPlainLanguage(language) {
		language = ""
	}
	return fence + language + "\n" + content + "\n" + fence
}

// isPlainLanguage reports whether a code block's language name holds only
// letters, digits and the punctuation that language names use.
func isPlainLanguage(language string) bool {
	for _, c := range language {
		isNameCharacter := c < unicode.MaxASCII && (unicode.IsLetter(c) || unicode.IsDigit(c) || strings.ContainsRune("+-_.#", c))
		if !isNameCharacter {
			return false
		}
	}
	return true
}

// list renders a list with "-" or numbered markers. Each item's further
// lines are indented by its marker's width, so they stay in the item.
func (r mattermostRenderer) list(node *ast.List) string {
	sep := "\n\n"
	if node.IsTight {
		sep = "\n"
	}
	var items []string
	number := node.Start
	for item := node.FirstChild(); item != nil; item = item.NextSibling() {
		marker := "- "
		if node.IsOrdered() {
			marker = fmt.Sprintf("%d. ", number)
			number++
		}
		content := r.joinBlocks(item, sep)
		if content == "" {
			items = append(items, strings.TrimSuffix(marker, " "))
			continue
		}
		items = append(items, marker+indentFollowingLines(content, len(marker)))
	}
	return strings.Join(items, sep)
}

// indentFollowingLines indents every line of text but the first by width
// spaces.
func indentFollowingLines(text string, width int) string {
	first, rest, hasMore := strings.Cut(text, "\n")
	if !hasMore {
		return text
	}
	return first + "\n" + prefixLines(rest, strings.Repeat(" ", width), "")
}

// htmlBlock shows raw HTML as text (ADR-037).
func (r mattermostRenderer) htmlBlock(node *ast.HTMLBlock) string {
	content := strings.TrimSuffix(node.Value.Str(r.source), "\n")
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = escapeText(line, true)
	}
	return strings.Join(lines, "\n")
}

// table renders a table with a header row, the alignment row and the body.
func (r mattermostRenderer) table(node *east.Table) string {
	var rows []string
	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		if body, isBody := row.(*east.TableBody); isBody {
			for bodyRow := body.FirstChild(); bodyRow != nil; bodyRow = bodyRow.NextSibling() {
				rows = append(rows, r.tableRow(bodyRow))
			}
			continue
		}
		rows = append(rows, r.tableRow(row))
		if _, isHeader := row.(*east.TableHeader); isHeader {
			rows = append(rows, alignmentRow(row))
		}
	}
	return strings.Join(rows, "\n")
}

// tableRow renders the cells of one row. A cell holds a single line, so a
// line break inside it becomes a space.
func (r mattermostRenderer) tableRow(row ast.Node) string {
	var cells []string
	for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
		content := strings.ReplaceAll(r.inlinesFrom(cell, false), "\\\n", " ")
		cells = append(cells, strings.ReplaceAll(content, "\n", " "))
	}
	return "| " + strings.Join(cells, " | ") + " |"
}

// alignmentRow renders the row below the header that aligns each column.
func alignmentRow(header ast.Node) string {
	var cells []string
	for cell := header.FirstChild(); cell != nil; cell = cell.NextSibling() {
		alignment := east.AlignNone
		if tableCell, ok := cell.(*east.TableCell); ok {
			alignment = tableCell.Alignment
		}
		switch alignment {
		case east.AlignLeft:
			cells = append(cells, ":---")
		case east.AlignCenter:
			cells = append(cells, ":---:")
		case east.AlignRight:
			cells = append(cells, "---:")
		default:
			cells = append(cells, "---")
		}
	}
	return "| " + strings.Join(cells, " | ") + " |"
}

// inlines renders the inline children of a block that starts a line.
func (r mattermostRenderer) inlines(parent ast.Node) string {
	return r.inlinesFrom(parent, true)
}

// inlinesFrom renders the inline children of parent. Text in a row of text
// nodes is escaped together, so markup split across nodes is still caught.
// atLineStart says whether the first child starts a line.
func (r mattermostRenderer) inlinesFrom(parent ast.Node, atLineStart bool) string {
	var out, pending strings.Builder
	flush := func() {
		if pending.Len() == 0 {
			return
		}
		out.WriteString(escapeText(pending.String(), atLineStart))
		pending.Reset()
		atLineStart = false
	}
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		textNode, isText := child.(*ast.Text)
		if !isText {
			flush()
			out.WriteString(r.inline(child))
			atLineStart = false
			continue
		}
		pending.WriteString(textNode.Value.Value(r.source))
		switch {
		case textNode.HardLineBreak():
			flush()
			out.WriteString("\\\n")
			atLineStart = true
		case textNode.SoftLineBreak():
			flush()
			out.WriteString("\n")
			atLineStart = true
		}
	}
	flush()
	return out.String()
}

// inline renders one inline node other than text.
func (r mattermostRenderer) inline(n ast.Node) string {
	switch node := n.(type) {
	case *Mention:
		return r.mention(node)
	case *ast.CodeSpan:
		return codeSpan(withoutMentionMarkup(node.Value.Value(r.source)))
	case *ast.Emphasis:
		return "*" + r.inlinesFrom(node, false) + "*"
	case *ast.Strong:
		return "**" + r.inlinesFrom(node, false) + "**"
	case *east.Strikethrough:
		return "~~" + r.inlinesFrom(node, false) + "~~"
	case *ast.Link:
		return r.link(r.inlinesFrom(node, false), node.Destination.Value(r.source))
	case *ast.Image:
		return r.link(r.inlinesFrom(node, false), node.Destination.Value(r.source))
	case *ast.AutoLink:
		return r.autoLink(node)
	case *ast.RawHTML:
		return escapeText(node.Value.Str(r.source), false)
	default:
		return r.inlinesFrom(node, false)
	}
}

// mention renders a mention as Mattermost's keyword. When text follows
// that Mattermost would read as part of the keyword's word, a word joiner
// ends the word, so the keyword stays whole.
func (r mattermostRenderer) mention(node *Mention) string {
	keyword := "@" + string(node.Target)
	next, isText := node.NextSibling().(*ast.Text)
	if !isText {
		return keyword
	}
	following, _ := utf8.DecodeRuneInString(next.Value.Value(r.source))
	if inMentionWord(following) {
		return keyword + wordJoiner
	}
	return keyword
}

// link renders a link, or only its text when the destination is not an
// http, https or mailto address (ADR-037).
func (r mattermostRenderer) link(text, destination string) string {
	target, ok := linkDestination(destination)
	if !ok {
		return text
	}
	if text == "" {
		text = escapeText(destination, false)
	}
	return "[" + text + "](" + target + ")"
}

// autoLink renders an autolink in angle brackets, or as text when its
// address is not an http, https or mailto address.
func (r mattermostRenderer) autoLink(node *ast.AutoLink) string {
	destination := node.Destination.Value(r.source)
	target, ok := linkDestination(destination)
	if !ok {
		return escapeText(node.Label.Value(r.source), false)
	}
	if email, isEmail := strings.CutPrefix(target, "mailto:"); isEmail {
		return "<" + email + ">"
	}
	return "<" + target + ">"
}

// linkDestination checks a link's destination and encodes it for Markdown.
func linkDestination(destination string) (string, bool) {
	address, err := url.Parse(destination)
	if err != nil {
		return "", false
	}
	switch address.Scheme {
	case "http", "https":
		if address.Host == "" {
			return "", false
		}
	case "mailto":
		if address.Opaque == "" {
			return "", false
		}
	default:
		return "", false
	}
	return encodeDestination(destination), true
}

// escapeText writes text so that Mattermost shows it as it is, without
// structure, links or mentions (SYSREQ-051).
func escapeText(text string, atLineStart bool) string {
	return breakAddresses(escapeCharacters(text, atLineStart))
}

// escapeCharacters escapes text one character at a time. Characters that
// open Markdown structure get a backslash, and those that open it only at
// the start of a line get one there. Entities keep "<", ">" and "&" from
// forming HTML or an entity. A word joiner follows "@", against a mention,
// and "~", against a channel link. Mention markup that no mention took goes.
func escapeCharacters(text string, atLineStart bool) string {
	var escaped strings.Builder
	runes := []rune(text)
	for i, c := range runes {
		switch {
		case strings.ContainsRune("\\`*_[]|", c):
			escaped.WriteString("\\" + string(c))
		case c == '<':
			escaped.WriteString("&lt;")
		case c == '>':
			escaped.WriteString("&gt;")
		case c == '&':
			escaped.WriteString("&amp;")
		case c == '@':
			escaped.WriteString("@" + wordJoiner)
		case c == '~':
			escaped.WriteString("\\~" + wordJoiner)
		case c == mentionOpen, c == mentionClose:
			// The markup of a mention that the parser did not take.
		case atLineStart && i == 0 && strings.ContainsRune("#+-=", c):
			escaped.WriteString("\\" + string(c))
		case atLineStart && endsLineStartNumber(runes, i):
			escaped.WriteString("\\" + string(c))
		default:
			escaped.WriteRune(c)
		}
	}
	return escaped.String()
}

// endsLineStartNumber reports whether index i holds the "." or ")" after
// the digits that start the text, which would start a numbered list.
func endsLineStartNumber(runes []rune, i int) bool {
	if i == 0 || (runes[i] != '.' && runes[i] != ')') {
		return false
	}
	for _, c := range runes[:i] {
		if !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}

// webHostStart matches a "www." where Mattermost would start a link: at the
// start of a word, in any case.
var webHostStart = regexp.MustCompile(`(?i)\bwww\.`)

// breakAddresses puts a word joiner into each "://" and "www.", so that
// Mattermost finds no address to link in plain text.
func breakAddresses(text string) string {
	text = strings.ReplaceAll(text, "://", ":"+wordJoiner+"//")
	return webHostStart.ReplaceAllStringFunc(text, func(start string) string {
		return start[:3] + wordJoiner + "."
	})
}
