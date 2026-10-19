// Package markdown turns the Markdown that message templates write into the
// format of each chat platform (ADR-037). Templates write CommonMark with the
// GitHub Flavored Markdown extensions for tables, strikethrough and
// autolinks. Forge text reaches them escaped, so it adds no structure of its
// own (SYSREQ-051), and a renderer per platform writes the parsed document
// in that platform's format.
package markdown

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// asciiPunctuation lists the characters that CommonMark lets a backslash
// escape (https://spec.commonmark.org/0.31.2/#ascii-punctuation-character).
const asciiPunctuation = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

// isASCIIPunctuation reports whether CommonMark lets a backslash escape c.
func isASCIIPunctuation(c rune) bool {
	return strings.ContainsRune(asciiPunctuation, c)
}

// Escape makes forge text, such as a merge request title, safe to place in
// a template (SYSREQ-051). Every ASCII punctuation character gets a
// backslash, so the text adds no emphasis, link, code or block of its own,
// and the parsed document holds it as plain text. Runs of white space,
// line breaks among them, become one space, and none stays at either end,
// where it would keep emphasis around the text from closing. Control
// characters and the mention markers go, so forge text cannot forge a
// mention.
func Escape(text string) string {
	var escaped strings.Builder
	inSpace := false
	for _, c := range text {
		switch {
		case unicode.IsSpace(c):
			if !inSpace {
				escaped.WriteByte(' ')
			}
			inSpace = true
			continue
		case unicode.IsControl(c), c == mentionOpen, c == mentionClose:
			continue
		case isASCIIPunctuation(c):
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(c)
		inSpace = false
	}
	return strings.TrimSpace(escaped.String())
}

// unescape reverses Escape's backslashes.
func unescape(escaped string) string {
	var plain strings.Builder
	runes := []rune(escaped)
	for i := 0; i < len(runes); i++ {
		escapesNext := runes[i] == '\\' && i+1 < len(runes) && isASCIIPunctuation(runes[i+1])
		if escapesNext {
			i++
		}
		plain.WriteRune(runes[i])
	}
	return plain.String()
}

// Code returns a code span that shows text that Escape escaped. A code span
// shows its content as it is, so the content goes in without backslashes,
// fenced by more backticks than it holds in a row.
func Code(escaped string) string {
	return codeSpan(unescape(escaped))
}

// codeSpan returns a code span that shows content as it is, or "" for no
// content.
func codeSpan(content string) string {
	if content == "" {
		return ""
	}
	fence := strings.Repeat("`", longestRun(content, '`')+1)
	if needsPadding(content) {
		content = " " + content + " "
	}
	return fence + content + fence
}

// longestRun counts the longest run of c in s.
func longestRun(s string, c rune) int {
	longest, current := 0, 0
	for _, r := range s {
		if r != c {
			current = 0
			continue
		}
		current++
		longest = max(longest, current)
	}
	return longest
}

// needsPadding reports whether a code span's content needs a space on each
// side: CommonMark strips one space from both ends, and a backtick at an end
// would join the fence.
func needsPadding(content string) bool {
	startsWithBacktick := strings.HasPrefix(content, "`")
	endsWithBacktick := strings.HasSuffix(content, "`")
	spaceOnBothEnds := strings.HasPrefix(content, " ") && strings.HasSuffix(content, " ")
	return startsWithBacktick || endsWithBacktick || spaceOnBothEnds
}

// LinkTarget returns an address from the forge as a link destination that
// templates may write inside <> or (). It returns "" for anything but an
// http or https address, so a forge cannot link to a script. The address is
// encoded anew, which escapes the characters that would end a destination.
func LinkTarget(raw string) string {
	address, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	isWeb := address.Scheme == "http" || address.Scheme == "https"
	if !isWeb || address.Host == "" {
		return ""
	}
	return encodeDestination(address.String())
}

// encodeDestination percent-encodes the characters that would end or break
// a link destination: spaces, angle brackets, parentheses, backslashes,
// backticks, quotes and control characters.
func encodeDestination(destination string) string {
	var encoded strings.Builder
	for _, b := range []byte(destination) {
		switch {
		case b <= ' ', b == 0x7f, strings.IndexByte("<>()\\`\"'", b) >= 0:
			fmt.Fprintf(&encoded, "%%%02X", b)
		default:
			encoded.WriteByte(b)
		}
	}
	return encoded.String()
}
