package deliver

import (
	"fmt"

	"github.com/patchcaster-app/patchcaster/internal/event"
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

// Renderer writes messages from the built-in templates, in Mattermost's
// format, the only chat platform so far (ADR-037).
type Renderer struct {
	templates map[message.Kind]*message.Template
}

// NewRenderer parses the built-in templates. A failure here is a build-time
// bug (the inputs are fixed), so callers should treat it as fatal at startup.
func NewRenderer() (*Renderer, error) {
	templates, err := message.Builtins()
	if err != nil {
		return nil, err
	}
	return &Renderer{templates: templates}, nil
}

// Root renders the root message from the current board snapshot: the same
// render serves announcement and every later refresh (SYSREQ-028), so a
// retried refresh is textually identical for unchanged state.
func (r *Renderer) Root(board *store.State) (string, error) {
	return r.render(message.Root, message.Facts{Board: board})
}

// Notice renders one thread notice by its plan name (internal/plan), about
// the event and the merge request's board.
func (r *Renderer) Notice(name string, board *store.State, ev *event.Event) (string, error) {
	kind, ok := noticeKinds[name]
	if !ok {
		return "", fmt.Errorf("no notice template %q", name)
	}
	return r.render(kind, message.Facts{Board: board, Event: ev})
}

// render writes the message of kind as Markdown, then in Mattermost's
// format.
func (r *Renderer) render(kind message.Kind, facts message.Facts) (string, error) {
	rendered, err := r.templates[kind].Render(facts)
	if err != nil {
		return "", fmt.Errorf("render %s message for MR %s: %w", kind, facts.Board.MrRef, err)
	}
	return markdown.Mattermost(markdown.Parse(rendered)), nil
}
