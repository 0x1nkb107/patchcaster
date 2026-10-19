package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

// A destination names its route and the scope of the route's source, so
// that ingest can pick the narrowest route of a sink (SYSREQ-049).
func TestADestinationNamesItsRouteAndScope(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)
	route, err := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	require.NoError(t, err)

	// act
	destinations, err := st.Destinations(ctx, l.paymentsRepo.ConnectionID, "payments/api")

	// assert
	require.NoError(t, err)
	require.Len(t, destinations, 1)
	assert.Equal(t, route.ID, destinations[0].RouteID)
	assert.Equal(t, "group", destinations[0].ScopeKind)
	assert.Equal(t, "payments", destinations[0].ScopePath)
}

// A queued message keeps the route it was accepted with, and a thread the
// route of its root (SYSREQ-049).
func TestAMessageAndItsThreadKeepTheirRoute(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	open := mrEvent("route-open", "open", nil)
	deliveries := toSinks(open, testSinkID)
	deliveries[0].RouteID = 21
	_, err := st.Accept(ctx, open, deliveries, now)
	require.NoError(t, err)
	sink := newSink(t, st, "chan-route")

	// act
	job, claimErr := st.ClaimDue(ctx, now, time.Minute)
	openErr := st.OpenThread(ctx, "42", sink.ID, "chan-route", "root-a", 21, now)
	withRoute, withErr := st.Thread(ctx, "42", sink.ID)
	withoutErr := st.OpenThread(ctx, "43", sink.ID, "chan-route", "root-b", 0, now)
	without, readErr := st.Thread(ctx, "43", sink.ID)

	// assert
	require.NoError(t, claimErr)
	require.NotNil(t, job)
	assert.Equal(t, int64(21), job.RouteID)
	require.NoError(t, openErr)
	require.NoError(t, withErr)
	assert.Equal(t, int64(21), withRoute.RouteID)
	require.NoError(t, withoutErr)
	require.NoError(t, readErr)
	assert.Zero(t, without.RouteID, "a root without a route keeps none")
}

// Delivery reads a route's templates, and learns when the route is gone.
func TestRouteTemplatesForTellsARemovedRoute(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)
	withTemplate, err := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	require.NoError(t, err)
	stored := store.RouteTemplate{RouteID: withTemplate.ID, Kind: "root", DataVersion: 1, Text: "own"}
	withTemplate, err = st.SetRouteTemplate(ctx, routingActor, stored, withTemplate.Version, time.Now())
	require.NoError(t, err)
	plain, err := createRoute(t, st, l.billing, l.billingRepo, l.billingRoom)
	require.NoError(t, err)

	// act
	templates, exists, readErr := st.RouteTemplatesFor(ctx, withTemplate.ID)
	none, plainExists, plainErr := st.RouteTemplatesFor(ctx, plain.ID)
	require.NoError(t, st.DeleteRoute(ctx, routingActor, withTemplate.ID, withTemplate.Version, time.Now()))
	gone, goneExists, goneErr := st.RouteTemplatesFor(ctx, withTemplate.ID)

	// assert
	require.NoError(t, readErr)
	assert.True(t, exists)
	assert.Equal(t, []store.RouteTemplate{stored}, templates)
	require.NoError(t, plainErr)
	assert.True(t, plainExists, "a route without templates of its own still exists")
	assert.Empty(t, none)
	require.NoError(t, goneErr)
	assert.False(t, goneExists)
	assert.Empty(t, gone)
}
