package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

// A route's template replaces the one it had, makes a new version of the
// route, and shows before and after in the audit log (SYSREQ-049,
// SCEN-016).
func TestARouteKeepsATemplatePerKind(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)
	route, err := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	require.NoError(t, err)
	first := store.RouteTemplate{RouteID: route.ID, Kind: "root", DataVersion: 1, Text: "first"}
	second := store.RouteTemplate{RouteID: route.ID, Kind: "root", DataVersion: 1, Text: "second"}

	// act
	withFirst, firstErr := st.SetRouteTemplate(ctx, routingActor, first, route.Version, time.Now())
	_, staleErr := st.SetRouteTemplate(ctx, routingActor, second, route.Version, time.Now())
	withSecond, secondErr := st.SetRouteTemplate(ctx, routingActor, second, withFirst.Version, time.Now())
	stored, readErr := st.RouteTemplate(ctx, route.ID, "root")
	entry, before, after := lastAudit(t, st)

	// assert
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	require.NoError(t, readErr)
	assert.ErrorIs(t, staleErr, store.ErrVersionMismatch)
	assert.Equal(t, route.Version+2, withSecond.Version)
	assert.Equal(t, []string{"root"}, withSecond.CustomTemplates)
	assert.Equal(t, &second, stored)
	assert.Equal(t, "route.template_set", entry.Action)
	assert.Equal(t, map[string]any{"kind": "root", "data_version": float64(1), "text": "first"}, before)
	assert.Equal(t, map[string]any{"kind": "root", "data_version": float64(1), "text": "second"}, after)
}

func TestARouteListsTheKindsItHasTemplatesFor(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)
	route, err := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	require.NoError(t, err)
	assert.Equal(t, []string{}, route.CustomTemplates, "a new route has none")

	// act
	version := route.Version
	for _, kind := range []string{"root", "comment"} {
		updated, err := st.SetRouteTemplate(ctx, routingActor, store.RouteTemplate{RouteID: route.ID, Kind: kind, DataVersion: 1, Text: kind}, version, time.Now())
		require.NoError(t, err)
		version = updated.Version
	}
	read, readErr := st.Route(ctx, route.ID)
	listed, listErr := st.Routes(ctx, 0, 0, 0, 10)

	// assert
	require.NoError(t, readErr)
	require.NoError(t, listErr)
	assert.Equal(t, []string{"comment", "root"}, read.CustomTemplates)
	require.Len(t, listed, 1)
	assert.Equal(t, []string{"comment", "root"}, listed[0].CustomTemplates)
}

func TestARemovedTemplateGivesWayToTheBuiltIn(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)
	route, err := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	require.NoError(t, err)
	template := store.RouteTemplate{RouteID: route.ID, Kind: "comment", DataVersion: 1, Text: "said"}
	withTemplate, err := st.SetRouteTemplate(ctx, routingActor, template, route.Version, time.Now())
	require.NoError(t, err)

	// act
	_, staleErr := st.DeleteRouteTemplate(ctx, routingActor, route.ID, "comment", route.Version, time.Now())
	without, deleteErr := st.DeleteRouteTemplate(ctx, routingActor, route.ID, "comment", withTemplate.Version, time.Now())
	entry, before, _ := lastAudit(t, st)
	_, readErr := st.RouteTemplate(ctx, route.ID, "comment")
	_, againErr := st.DeleteRouteTemplate(ctx, routingActor, route.ID, "comment", without.Version, time.Now())

	// assert
	assert.ErrorIs(t, staleErr, store.ErrVersionMismatch)
	require.NoError(t, deleteErr)
	assert.Equal(t, []string{}, without.CustomTemplates)
	assert.Equal(t, withTemplate.Version+1, without.Version)
	assert.Equal(t, "route.template_delete", entry.Action)
	assert.Equal(t, "said", before["text"])
	assert.ErrorIs(t, readErr, store.ErrNotFound)
	assert.ErrorIs(t, againErr, store.ErrNotFound)
}

func TestTemplatesGoWithTheirRoute(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)
	route, err := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	require.NoError(t, err)
	withTemplate, err := st.SetRouteTemplate(ctx, routingActor, store.RouteTemplate{RouteID: route.ID, Kind: "root", DataVersion: 1, Text: "x"}, route.Version, time.Now())
	require.NoError(t, err)

	// act
	deleteErr := st.DeleteRoute(ctx, routingActor, route.ID, withTemplate.Version, time.Now())
	_, readErr := st.RouteTemplate(ctx, route.ID, "root")
	_, setErr := st.SetRouteTemplate(ctx, routingActor, store.RouteTemplate{RouteID: route.ID, Kind: "root", DataVersion: 1, Text: "y"}, withTemplate.Version, time.Now())

	// assert
	require.NoError(t, deleteErr)
	assert.ErrorIs(t, readErr, store.ErrNotFound)
	assert.ErrorIs(t, setErr, store.ErrNotFound)
}
