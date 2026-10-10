package deliver

import (
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/0x5a17ed/patchcaster/internal/event"
)

// Templates are embedded in the binary (SYSREQ-025); there is no operator-
// supplied template path to load.
//
//go:embed templates/*.tmpl
var templateFS embed.FS

// mdReplacer neutralizes Mattermost Markdown structure in field values so a
// title cannot forge links, mentions, or layout (SYSREQ-003). Backslash
// escaping is Mattermost's own syntax for emphasis and bracket characters;
// angle brackets become entities because a bare "\<" is not safe there.
var mdReplacer = strings.NewReplacer(
	"`", "'",
	"*", "\\*",
	"_", "\\_",
	"~", "\\~",
	"[", "\\[",
	"]", "\\]",
	"<", "&lt;",
	">", "&gt;",
	"\r", " ",
	"\n", " ",
)

// safeURL strips characters that could break out of a bare-line URL.
func safeURL(u string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r <= ' ', r == 0x7f, r == '<', r == '>':
			return -1
		default:
			return r
		}
	}, u)
}

// View is the only thing a template may touch: every field is escaped or
// sanitized at construction, so no accessor leaks raw payload text.
type View struct {
	Title       string
	ProjectPath string
	Author      string
	Actor       string
	Action      string
	IID         string
	URL         string
}

// NewView builds the escaped projection of an event.
func NewView(ev *event.Event) *View {
	return &View{
		Title:       mdReplacer.Replace(ev.Title),
		ProjectPath: mdReplacer.Replace(ev.ProjectPath),
		Author:      mdReplacer.Replace(ev.Author),
		Actor:       mdReplacer.Replace(ev.Actor),
		Action:      mdReplacer.Replace(ev.Action),
		IID:         fmt.Sprintf("%d", ev.IID),
		URL:         safeURL(ev.URL),
	}
}

// Renderer executes the embedded templates against escaped views.
type Renderer struct {
	root  *template.Template
	reply *template.Template
}

// NewRenderer parses the embedded templates. A failure here is a build-time
// bug (the inputs are fixed), so callers should treat it as fatal at startup.
func NewRenderer() (*Renderer, error) {
	root, err := template.ParseFS(templateFS, "templates/root.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse root template: %w", err)
	}
	reply, err := template.ParseFS(templateFS, "templates/reply.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse reply template: %w", err)
	}
	return &Renderer{root: root, reply: reply}, nil
}

// Root renders the thread-opening post for an "open" event.
func (r *Renderer) Root(ev *event.Event) (string, error) {
	return r.exec(r.root, "root.tmpl", ev)
}

// Reply renders a thread reply for any other allowlisted action.
func (r *Renderer) Reply(ev *event.Event) (string, error) {
	return r.exec(r.reply, "reply.tmpl", ev)
}

func (r *Renderer) exec(t *template.Template, name string, ev *event.Event) (string, error) {
	var sb strings.Builder
	if err := t.Execute(&sb, NewView(ev)); err != nil {
		return "", fmt.Errorf("execute %s for MR %s: %w", name, ev.MRRef, err)
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
