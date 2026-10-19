package deliver

import (
	"embed"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// Templates are embedded in the binary (SYSREQ-025); there is no template
// path for the infrastructure team to set.
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
	"@", "\\@", // titles demanding "@here" must not ping the channel (SYSREQ-003)
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

// RootView is the escaped projection of the board a root message renders
// from; the status line arrives precomputed (StatusLine). Every field is
// sanitized at construction, so no template can leak raw payload text.
type RootView struct {
	IID         string
	Title       string
	Author      string
	ProjectPath string
	Source      string
	Target      string
	URL         string
	Status      string
}

// NewRootView builds the escaped projection of a board snapshot.
func NewRootView(st *store.State) *RootView {
	return &RootView{
		IID:         strconv.FormatInt(st.IID, 10),
		Title:       mdReplacer.Replace(st.Title),
		Author:      mdReplacer.Replace(st.Author),
		ProjectPath: mdReplacer.Replace(st.ProjectPath),
		Source:      mdReplacer.Replace(st.SourceBranch),
		Target:      mdReplacer.Replace(st.TargetBranch),
		URL:         safeURL(st.URL),
		Status:      StatusLine(st),
	}
}

// NoticeView is the escaped projection a thread notice renders from (the
// acting user and merge request number — never payload bodies).
type NoticeView struct {
	Actor   string
	IID     string
	Commits string // preformatted commit count for push notices, else ""
}

// NewNoticeView builds the escaped projection of an event for a notice.
func NewNoticeView(ev *event.Event) *NoticeView {
	v := &NoticeView{
		Actor: mdReplacer.Replace(ev.Actor),
		IID:   strconv.FormatInt(ev.IID, 10),
	}
	if ev.PushCount != nil && *ev.PushCount > 0 {
		v.Commits = strconv.Itoa(*ev.PushCount)
	}
	return v
}

// plural renders a count with a singular noun at one, plural otherwise.
// (Mattermost threads favor compact labels; "1 comment" over "1 comments".)
func plural(n int, singular, suffix string) string {
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + singular + suffix
}

// StatusLine assembles the root's second line (SYSREQ-028): segments joined
// by " • ", each present only when its signal exists; empty when none is.
func StatusLine(st *store.State) string {
	var segments []string
	if st.PipelineStatus != "" {
		segments = append(segments, pipelineSegment(st.PipelineStatus))
	}
	if st.ReviewersRequired > 0 {
		segments = append(segments, fmt.Sprintf(
			"👀 %d/%d reviewers", st.ReviewersDone, st.ReviewersRequired))
	}
	if st.Comments > 0 {
		segments = append(segments, "💬 "+plural(st.Comments, "comment", "s"))
	}
	if st.Edits > 0 {
		segments = append(segments, "📝 "+plural(st.Edits, "edit", "s"))
	}
	return strings.Join(segments, " • ")
}

// pipelineSegment maps GitLab pipeline statuses onto the owner's fixed
// emoji table (2026-10-10); unknown statuses degrade to a neutral marker
// with the raw status text — escaped — and never fail the render.
func pipelineSegment(status string) string {
	switch status {
	case "running":
		return "🟡 Pipeline running"
	case "success":
		return "🟢 Pipeline passed"
	case "failed":
		return "🔴 Pipeline failed"
	case "waiting_for_resource", "scheduled", "prepared", "waiting", "manual":
		return "⚪ Pipeline waiting"
	case "canceled":
		return "⚪ Pipeline canceled"
	case "skipped":
		return "⚪ Pipeline skipped"
	default:
		return "⚪ Pipeline " + mdReplacer.Replace(status)
	}
}

// Renderer executes the embedded templates against escaped views.
type Renderer struct {
	root    *template.Template
	noticeT *template.Template
}

// NewRenderer parses the embedded templates. A failure here is a build-time
// bug (the inputs are fixed), so callers should treat it as fatal at startup.
func NewRenderer() (*Renderer, error) {
	root, err := template.ParseFS(templateFS, "templates/root.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse root template: %w", err)
	}
	notices, err := template.ParseFS(templateFS, "templates/notice_*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse notice templates: %w", err)
	}
	return &Renderer{root: root, noticeT: notices}, nil
}

// Root renders the root message from the current board snapshot: the same
// render serves announcement and every later refresh (SYSREQ-028), so a
// retried refresh is textually identical for unchanged state.
func (r *Renderer) Root(st *store.State) (string, error) {
	return exec(r.root, "root.tmpl", NewRootView(st), st.MrRef)
}

// Notice renders one thread notice by plan name (internal/plan); the name
// maps to templates/notice_<name>.tmpl.
func (r *Renderer) Notice(name string, ev *event.Event) (string, error) {
	file := "notice_" + name + ".tmpl"
	t := r.noticeT.Lookup(file) // ParseFS names templates by file basename.
	if t == nil {
		return "", fmt.Errorf("no notice template %q", file)
	}
	return exec(t, file, NewNoticeView(ev), ev.MRRef)
}

func exec(t *template.Template, name string, view any, mrRef string) (string, error) {
	var sb strings.Builder
	if err := t.Execute(&sb, view); err != nil {
		return "", fmt.Errorf("execute %s for MR %s: %w", name, mrRef, err)
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
