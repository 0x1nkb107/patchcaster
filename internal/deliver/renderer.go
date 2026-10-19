package deliver

import (
	"fmt"

	"github.com/patchcaster-app/patchcaster/internal/markdown"
	"github.com/patchcaster-app/patchcaster/internal/message"
	"github.com/patchcaster-app/patchcaster/internal/plan"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// noticeKinds maps the notice names of delivery plans (internal/plan) to
// the kinds of messages. Plans are stored with queued messages, so their
// names stay as they are.
var noticeKinds = map[string]message.Kind{
	plan.Comment:      message.Comment,
	plan.PipelinePass: message.PipelinePassed,
	plan.PipelineFail: message.PipelineFailed,
	plan.Reviewed:     message.Reviewed,
	plan.Changes:      message.ChangesRequested,
	plan.Approval:     message.Approved,
	plan.Push:         message.Pushed,
	plan.Merged:       message.Merged,
	plan.Closed:       message.Closed,
}

// noticeKind returns the kind of message of a plan's notice.
func noticeKind(name string) (message.Kind, error) {
	kind, ok := noticeKinds[name]
	if !ok {
		return "", fmt.Errorf("no notice template %q", name)
	}
	return kind, nil
}

// Renderer writes messages in Mattermost's format, the only chat platform
// so far (ADR-037), with the built-in templates or with a route's own.
type Renderer struct {
	builtins map[message.Kind]*message.Template
}

// NewRenderer parses the built-in templates. A failure here is a build-time
// bug (the inputs are fixed), so callers should treat it as fatal at startup.
func NewRenderer() (*Renderer, error) {
	builtins, err := message.Builtins()
	if err != nil {
		return nil, err
	}
	return &Renderer{builtins: builtins}, nil
}

// Builtin writes the message of kind with its built-in template. The same
// board renders the same text, so a retried root refresh is identical
// (SYSREQ-028).
func (r *Renderer) Builtin(kind message.Kind, facts message.Facts) (string, error) {
	return render(r.builtins[kind], facts)
}

// Stored writes a message with a route's own template (SYSREQ-049). The
// template passed its check when it was stored, but a later release may
// read it differently, so it is parsed anew.
func (r *Renderer) Stored(stored store.RouteTemplate, facts message.Facts) (string, error) {
	template, err := message.Parse(message.Kind(stored.Kind), message.Version(stored.DataVersion), stored.Text)
	if err != nil {
		return "", fmt.Errorf("parse the %s template of route %d: %w", stored.Kind, stored.RouteID, err)
	}
	return render(template, facts)
}

// render writes the message as Markdown, then in Mattermost's format.
func render(template *message.Template, facts message.Facts) (string, error) {
	rendered, err := template.Render(facts)
	if err != nil {
		return "", fmt.Errorf("render %s message for MR %s: %w", template.Kind(), facts.Board.MrRef, err)
	}
	return markdown.Mattermost(markdown.Parse(rendered)), nil
}
