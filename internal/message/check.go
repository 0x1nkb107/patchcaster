package message

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/markdown"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// The longest forge values that the size check assumes. Titles, usernames
// and branch names are 255 characters long at most in GitLab. Project paths
// and addresses get more room, since they nest groups.
const (
	longestName    = 255
	longestPath    = 1024
	longestAddress = 2048
)

// sample is a set of facts that every template must render. A sample for
// the size check only is no example for people.
type sample struct {
	description string
	facts       Facts
	sizeOnly    bool
}

// sampleBoard is an open merge request without any status yet.
func sampleBoard() *store.State {
	return &store.State{
		MrRef:        "sample",
		IID:          12,
		Title:        "Add flux capacitor",
		Author:       "ada",
		ProjectPath:  "grp/app",
		SourceBranch: "feature/flux",
		TargetBranch: "main",
		URL:          "https://gitlab.example/grp/app/-/merge_requests/12",
		Open:         true,
	}
}

// longestBoard holds the longest values the size check assumes. Each is
// made of "&", whose escaping grows most.
func longestBoard() *store.State {
	board := sampleBoard()
	board.Title = strings.Repeat("&", longestName)
	board.Author = strings.Repeat("&", longestName)
	board.ProjectPath = strings.Repeat("&", longestPath)
	board.SourceBranch = strings.Repeat("&", longestName)
	board.TargetBranch = strings.Repeat("&", longestName)
	board.URL = "https://gitlab.example/" + strings.Repeat("(", (longestAddress-len("https://gitlab.example/"))/3)
	board.PipelineStatus = strings.Repeat("&", longestName)
	board.ReviewersDone, board.ReviewersRequired = 99, 99
	board.Comments, board.Edits = 9999, 9999
	return board
}

// sampleBoards returns boards in each state a template may test: every
// pipeline state, with and without reviews and counts, open and closed,
// without an address, and with the longest values.
func sampleBoards() []sample {
	samples := []sample{{"an open merge request", Facts{Board: sampleBoard()}, false}}
	for _, status := range []string{"running", "success", "failed", "pending", "canceled", "skipped", "unheard-of"} {
		board := sampleBoard()
		board.PipelineStatus = status
		board.ReviewersDone, board.ReviewersRequired = 1, 2
		board.Comments, board.Edits = 3, 1
		samples = append(samples, sample{fmt.Sprintf("a pipeline %s", pipelineState(status)), Facts{Board: board}, false})
	}
	closed := sampleBoard()
	closed.Open = false
	withoutAddress := sampleBoard()
	withoutAddress.URL = ""
	return append(samples,
		sample{"a closed merge request", Facts{Board: closed}, false},
		sample{"a merge request without an address", Facts{Board: withoutAddress}, false},
		sample{"the longest values", Facts{Board: longestBoard()}, true},
	)
}

// samplesOf returns the samples for messages of kind. A notice gets an
// event with each board, with and without a commit count, and the longest
// actor with the longest board.
func samplesOf(kind Kind) []sample {
	boards := sampleBoards()
	if !kind.IsNotice() {
		return boards
	}
	commits := 3
	var samples []sample
	for _, board := range boards {
		actor := "bob"
		if board.sizeOnly {
			actor = strings.Repeat("&", longestName)
		}
		samples = append(samples,
			sample{board.description, Facts{Board: board.facts.Board, Event: &event.Event{Actor: actor}}, board.sizeOnly},
			sample{board.description + " and a commit count", Facts{Board: board.facts.Board, Event: &event.Event{Actor: actor, PushCount: &commits}}, board.sizeOnly},
		)
	}
	return samples
}

// Check renders every sample with t (ADR-038). It refuses t when a sample
// fails, naming the line, and when a message would be longer than a chat
// platform accepts.
func Check(t *Template) error {
	for _, sample := range samplesOf(t.kind) {
		rendered, err := t.Render(sample.facts)
		if err != nil {
			return withSample(err, sample)
		}
		length := utf8.RuneCountInString(markdown.Mattermost(markdown.Parse(rendered)))
		if length > markdown.MattermostMaxRunes {
			return &Error{Cause: fmt.Sprintf("with %s the message holds %d characters, more than the %d that Mattermost accepts",
				sample.description, length, markdown.MattermostMaxRunes)}
		}
	}
	return nil
}

// withSample names the sample that a template failed on.
func withSample(err error, sample sample) error {
	var refusal *Error
	if !errors.As(err, &refusal) {
		return err
	}
	return &Error{Line: refusal.Line, Cause: fmt.Sprintf("with %s: %s", sample.description, refusal.Cause)}
}

// Rendering is the message a template wrote about one sample, as Markdown.
type Rendering struct {
	Description string
	Markdown    string
}

// Preview checks t and renders the samples that serve as examples: all
// but those of the size check (SCEN-021).
func Preview(t *Template) ([]Rendering, error) {
	if err := Check(t); err != nil {
		return nil, err
	}
	var renderings []Rendering
	for _, sample := range samplesOf(t.kind) {
		if sample.sizeOnly {
			continue
		}
		rendered, err := t.Render(sample.facts)
		if err != nil {
			return nil, withSample(err, sample)
		}
		renderings = append(renderings, Rendering{Description: sample.description, Markdown: rendered})
	}
	return renderings, nil
}
