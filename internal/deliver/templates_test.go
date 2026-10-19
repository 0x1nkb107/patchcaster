package deliver

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/plan"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// The routes of these tests: the one a job was queued with, and the one
// its thread started with.
const (
	queuedRoute = 21
	threadRoute = 22
)

// template is a route's own template of kind.
func template(routeID int64, kind, text string) store.RouteTemplate {
	return store.RouteTemplate{RouteID: routeID, Kind: kind, DataVersion: 1, Text: text}
}

// A root is written with the template of the route its job was queued
// with, and its thread remembers that route (SYSREQ-049).
func TestWorker_RootUsesTheQueuedRoutesTemplate(t *testing.T) {
	// arrange
	h := newHarness(t)
	h.fs.routes = map[int64][]store.RouteTemplate{
		queuedRoute: {template(queuedRoute, "root", "Opened: **{{.MergeRequest.Title}}**")},
	}
	job := openJob(0)
	job.RouteID = queuedRoute

	// act
	h.w.process(context.Background(), job)

	// assert
	require.Len(t, h.fp.calls, 1)
	assert.Equal(t, "Opened: **T**", h.fp.calls[0].message)
	require.NotNil(t, h.fs.thread)
	assert.Equal(t, int64(queuedRoute), h.fs.thread.RouteID)
}

// A thread keeps the look of its root: its notices and refreshes use the
// route the thread started with, not the one a later job was queued with.
func TestWorker_AThreadKeepsItsRoutesTemplates(t *testing.T) {
	// arrange
	h := newHarness(t)
	openBoardThread(h)
	h.fs.thread.RouteID = threadRoute
	h.fs.routes = map[int64][]store.RouteTemplate{
		threadRoute: {
			template(threadRoute, "comment", "{{.Event.Actor}} said something"),
			template(threadRoute, "root", "!{{.MergeRequest.Number}} with {{.Status.Comments}} comments"),
		},
		queuedRoute: {template(queuedRoute, "comment", "not this one")},
	}
	job := planJob(3, 0, &event.Plan{Notice: plan.Comment, Refresh: true})
	job.RouteID = queuedRoute

	// act
	h.w.process(context.Background(), job)

	// assert
	require.Len(t, h.fp.calls, 1)
	assert.Equal(t, "bob said something", h.fp.calls[0].message)
	require.Len(t, h.fp.updates, 1)
	assert.Equal(t, "!12 with 3 comments", h.fp.updates[0].message)
}

// When the thread's route is gone, the route the job was queued with
// writes its messages; a kind it has no template for stays built in.
func TestWorker_TheQueuedRouteTakesOverFromARemovedOne(t *testing.T) {
	// arrange
	h := newHarness(t)
	openBoardThread(h)
	h.fs.thread.RouteID = threadRoute
	h.fs.routes = map[int64][]store.RouteTemplate{
		queuedRoute: {template(queuedRoute, "comment", "{{.Event.Actor}} commented here")},
	}
	job := planJob(3, 0, &event.Plan{Notice: plan.Comment, Refresh: true})
	job.RouteID = queuedRoute

	// act
	h.w.process(context.Background(), job)

	// assert
	require.Len(t, h.fp.calls, 1)
	assert.Equal(t, "bob commented here", h.fp.calls[0].message)
	require.Len(t, h.fp.updates, 1)
	assert.Equal(t, wantRefreshedRoot, h.fp.updates[0].message, "the route has no root template of its own")
}

// A route's template that fails at delivery gives way to the built-in one,
// and the fallback is counted (owner decision 2026-10-20).
func TestWorker_AFailingTemplateFallsBackToTheBuiltIn(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"a render failure", "{{.Nope}}"},
		{"a template that no longer parses", "{{printf \"%s\" .Event.Actor}}"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			h := newHarness(t)
			openBoardThread(h)
			h.fs.thread.RouteID = threadRoute
			h.fs.routes = map[int64][]store.RouteTemplate{
				threadRoute: {template(threadRoute, "comment", tc.text)},
			}

			// act
			h.w.process(context.Background(), noticeJob(3, 0, "chan-dest"))

			// assert
			require.Len(t, h.fp.calls, 1)
			assert.Equal(t, "💬 bob commented", h.fp.calls[0].message)
			assert.Equal(t, 1.0, testutil.ToFloat64(h.m.TemplateFallbacks.WithLabelValues("comment", testChatLabel, testSinkLabel)))
			require.Len(t, h.fs.settles, 1)
			assert.Equal(t, store.SettlementSent, h.fs.settles[0].s.Kind)
		})
	}
}

// A store fault while reading the templates retries the job instead of
// writing it with the wrong look.
func TestWorker_ATemplateLookupFaultRetries(t *testing.T) {
	// arrange
	h := newHarness(t)
	openBoardThread(h)
	h.fs.thread.RouteID = threadRoute
	h.fs.errRoutes = assert.AnError

	// act
	h.w.process(context.Background(), noticeJob(3, 0, "chan-dest"))

	// assert
	assert.Empty(t, h.fp.calls)
	require.Len(t, h.fs.settles, 1)
	assert.Equal(t, store.SettlementRetried, h.fs.settles[0].s.Kind)
}
