package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

var routingActor = store.Actor{Label: "ada"}

// layout is a routing setup: two teams, each with its own source and sink,
// and a manager for the first.
type layout struct {
	payments, billing         *store.Team
	paymentsRepo, billingRepo *store.Source
	paymentsRoom, billingRoom *store.Sink
	lead                      *store.Person
}

// newLayout builds a layout on st.
func newLayout(t *testing.T, st *store.Postgres) layout {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	forge := createConnection(t, st, "gitlab", "gitlab", nil)
	chat := createConnection(t, st, "mattermost", "chat", []byte("sealed"))
	lead, err := st.RecordLogin(ctx, login("lead"), now)
	require.NoError(t, err)
	l := layout{lead: lead}
	l.paymentsRepo = createSource(t, st, forge, "payments", "group", "payments")
	l.billingRepo = createSource(t, st, forge, "billing", "repository", "billing/api")
	l.paymentsRoom = createSink(t, st, chat, "payments room", "c-payments")
	l.billingRoom = createSink(t, st, chat, "billing room", "c-billing")
	l.payments = createTeam(t, st, "Payments", []int64{lead.ID}, l.paymentsRepo, l.paymentsRoom)
	l.billing = createTeam(t, st, "Billing", nil, l.billingRepo, l.billingRoom)
	return l
}

func createSource(t *testing.T, st *store.Postgres, forge *store.Connection, name, kind, path string) *store.Source {
	t.Helper()
	source, err := st.CreateSource(context.Background(), routingActor, store.Source{
		ConnectionID: forge.ID,
		Name:         name,
		ScopeKind:    kind,
		ScopePath:    path,
	}, time.Now())
	require.NoError(t, err)
	return source
}

func createSink(t *testing.T, st *store.Postgres, chat *store.Connection, name, channel string) *store.Sink {
	t.Helper()
	sink, err := st.CreateSink(context.Background(), routingActor, store.Sink{
		ConnectionID: chat.ID,
		Name:         name,
		ChannelID:    channel,
		ChannelName:  name,
	}, time.Now())
	require.NoError(t, err)
	return sink
}

// createTeam creates a team and gives it managers, a source and a sink.
func createTeam(t *testing.T, st *store.Postgres, name string, managers []int64, source *store.Source, sink *store.Sink) *store.Team {
	t.Helper()
	ctx := context.Background()
	team, err := st.CreateTeam(ctx, routingActor, name, time.Now())
	require.NoError(t, err)
	team.Managers = managers
	team.Sources = []int64{source.ID}
	team.Sinks = []int64{sink.ID}
	team, err = st.UpdateTeam(ctx, routingActor, *team, time.Now())
	require.NoError(t, err)
	return team
}

func createRoute(t *testing.T, st *store.Postgres, team *store.Team, source *store.Source, sink *store.Sink) (*store.Route, error) {
	t.Helper()
	return st.CreateRoute(context.Background(), routingActor, store.Route{
		TeamID:   team.ID,
		SourceID: source.ID,
		SinkID:   sink.ID,
	}, time.Now())
}

// requireFieldError asserts that err names field and wraps want.
func requireFieldError(t *testing.T, err error, field string, want error) {
	t.Helper()
	var fieldErr *store.FieldError
	require.ErrorAs(t, err, &fieldErr)
	assert.Equal(t, field, fieldErr.Field)
	assert.ErrorIs(t, err, want)
}

func TestATeamOwnsWhatItIsGiven(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	l := newLayout(t, st)

	// act
	read, err := st.Team(context.Background(), l.payments.ID)
	manages, managesErr := st.ManagesTeam(context.Background(), l.lead.ID, l.payments.ID)
	managesOther, _ := st.ManagesTeam(context.Background(), l.lead.ID, l.billing.ID)

	// assert
	require.NoError(t, err)
	assert.Equal(t, []int64{l.lead.ID}, read.Managers)
	assert.Equal(t, []int64{l.paymentsRepo.ID}, read.Sources)
	assert.Equal(t, []int64{l.paymentsRoom.ID}, read.Sinks)
	require.NoError(t, managesErr)
	assert.True(t, manages)
	assert.False(t, managesOther)
}

func TestATeamRefusesUnknownMembers(t *testing.T) {
	tests := []struct {
		field  string
		change func(*store.Team)
	}{
		{"managers", func(team *store.Team) { team.Managers = []int64{9999} }},
		{"sources", func(team *store.Team) { team.Sources = []int64{9999} }},
		{"sinks", func(team *store.Team) { team.Sinks = []int64{9999} }},
	}
	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			// arrange
			st, _ := openTestStore(t)
			l := newLayout(t, st)
			changed := *l.billing
			tc.change(&changed)

			// act
			_, err := st.UpdateTeam(context.Background(), routingActor, changed, time.Now())
			after, _ := st.Team(context.Background(), l.billing.ID)

			// assert
			requireFieldError(t, err, tc.field, store.ErrUnknownReference)
			assert.Equal(t, l.billing, after, "a refused change leaves the team as it was")
		})
	}
}

// A route connects only a source and a sink of its own team (SYSREQ-046).
func TestARouteStaysInsideItsTeam(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	l := newLayout(t, st)

	// act
	route, okErr := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	_, foreignSourceErr := createRoute(t, st, l.payments, l.billingRepo, l.paymentsRoom)
	_, foreignSinkErr := createRoute(t, st, l.payments, l.paymentsRepo, l.billingRoom)
	_, twiceErr := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)

	// assert
	require.NoError(t, okErr)
	assert.Empty(t, route.MutedReplies)
	requireFieldError(t, foreignSourceErr, "source_id", store.ErrNotTheTeams)
	requireFieldError(t, foreignSinkErr, "sink_id", store.ErrNotTheTeams)
	requireFieldError(t, twiceErr, "sink_id", store.ErrDuplicate)
}

// What a route uses cannot be taken away underneath it.
func TestWhatARouteUsesStays(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)
	_, err := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	require.NoError(t, err)
	withoutSource := *l.payments
	withoutSource.Sources = nil
	withoutSink := *l.payments
	withoutSink.Sinks = nil
	forge, err := st.Connection(ctx, l.paymentsRepo.ConnectionID)
	require.NoError(t, err)

	// act
	_, sourceErr := st.UpdateTeam(ctx, routingActor, withoutSource, time.Now())
	_, sinkErr := st.UpdateTeam(ctx, routingActor, withoutSink, time.Now())
	teamErr := st.DeleteTeam(ctx, routingActor, l.payments.ID, l.payments.Version, time.Now())
	deleteSourceErr := st.DeleteSource(ctx, routingActor, l.paymentsRepo.ID, l.paymentsRepo.Version, time.Now())
	deleteSinkErr := st.DeleteSink(ctx, routingActor, l.paymentsRoom.ID, l.paymentsRoom.Version, time.Now())
	connectionErr := st.DeleteConnection(ctx, routingActor, forge.ID, forge.Version, time.Now())

	// assert
	requireFieldError(t, sourceErr, "sources", store.ErrInUse)
	requireFieldError(t, sinkErr, "sinks", store.ErrInUse)
	assert.ErrorIs(t, teamErr, store.ErrInUse)
	assert.ErrorIs(t, deleteSourceErr, store.ErrInUse)
	assert.ErrorIs(t, deleteSinkErr, store.ErrInUse)
	assert.ErrorIs(t, connectionErr, store.ErrInUse, "a connection with sources stays")
}

func TestUnusedItemsGo(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)
	route, err := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	require.NoError(t, err)

	// act
	routeErr := st.DeleteRoute(ctx, routingActor, route.ID, route.Version, time.Now())
	sourceErr := st.DeleteSource(ctx, routingActor, l.paymentsRepo.ID, l.paymentsRepo.Version, time.Now())
	team, _ := st.Team(ctx, l.payments.ID)
	teamErr := st.DeleteTeam(ctx, routingActor, team.ID, team.Version, time.Now())

	// assert
	require.NoError(t, routeErr)
	require.NoError(t, sourceErr, "a source without routes goes, with its team assignments")
	assert.Empty(t, team.Sources)
	require.NoError(t, teamErr)
	assert.Subset(t, auditActions(t, st), []string{"route.delete", "source.delete", "team.delete"})
}

// A manager sees the teams they manage and what those own; an
// administrator sees everything (SYSREQ-039).
func TestManagersSeeOnlyTheirTeams(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)
	_, err := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	require.NoError(t, err)
	_, err = createRoute(t, st, l.billing, l.billingRepo, l.billingRoom)
	require.NoError(t, err)

	// act
	teams, _ := st.Teams(ctx, l.lead.ID, 0, 10)
	sources, _ := st.Sources(ctx, l.lead.ID, 0, 10)
	sinks, _ := st.Sinks(ctx, l.lead.ID, 0, 10)
	routes, _ := st.Routes(ctx, l.lead.ID, 0, 0, 10)
	allRoutes, _ := st.Routes(ctx, 0, 0, 0, 10)
	billingRoutes, _ := st.Routes(ctx, 0, l.billing.ID, 0, 10)

	// assert
	require.Len(t, teams, 1)
	assert.Equal(t, l.payments.ID, teams[0].ID)
	require.Len(t, sources, 1)
	assert.Equal(t, l.paymentsRepo.ID, sources[0].ID)
	require.Len(t, sinks, 1)
	assert.Equal(t, l.paymentsRoom.ID, sinks[0].ID)
	require.Len(t, routes, 1)
	assert.Equal(t, l.payments.ID, routes[0].TeamID)
	assert.Len(t, allRoutes, 2)
	require.Len(t, billingRoutes, 1)
	assert.Equal(t, l.billing.ID, billingRoutes[0].TeamID)
}

func TestASharedSinkServesSeveralTeams(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)
	shared := *l.billing
	shared.Sinks = []int64{l.billingRoom.ID, l.paymentsRoom.ID}
	_, err := st.UpdateTeam(ctx, routingActor, shared, time.Now())
	require.NoError(t, err)

	// act
	_, paymentsErr := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	_, billingErr := createRoute(t, st, l.billing, l.billingRepo, l.paymentsRoom)
	room, _ := st.Sink(ctx, l.paymentsRoom.ID)

	// assert
	require.NoError(t, paymentsErr)
	require.NoError(t, billingErr)
	assert.ElementsMatch(t, []int64{l.payments.ID, l.billing.ID}, room.Teams)
}

func TestDuplicatesAreRefusedByTheirField(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)

	// act
	_, nameErr := st.CreateSource(ctx, routingActor, store.Source{
		ConnectionID: l.paymentsRepo.ConnectionID,
		Name:         "payments",
		ScopeKind:    "group",
		ScopePath:    "other",
	}, time.Now())
	_, scopeErr := st.CreateSource(ctx, routingActor, store.Source{
		ConnectionID: l.paymentsRepo.ConnectionID,
		Name:         "payments again",
		ScopeKind:    "group",
		ScopePath:    "payments",
	}, time.Now())
	_, channelErr := st.CreateSink(ctx, routingActor, store.Sink{
		ConnectionID: l.paymentsRoom.ConnectionID,
		Name:         "again",
		ChannelID:    "c-payments",
	}, time.Now())
	_, connectionErr := st.CreateSink(ctx, routingActor, store.Sink{
		ConnectionID: 9999,
		Name:         "nowhere",
		ChannelID:    "c",
	}, time.Now())
	_, teamErr := st.CreateTeam(ctx, routingActor, "Payments", time.Now())

	// assert
	requireFieldError(t, nameErr, "name", store.ErrNameTaken)
	requireFieldError(t, scopeErr, "scope", store.ErrDuplicate)
	requireFieldError(t, channelErr, "channel_id", store.ErrDuplicate)
	requireFieldError(t, connectionErr, "connection_id", store.ErrUnknownReference)
	assert.ErrorIs(t, teamErr, store.ErrNameTaken)
}

func TestARouteMutesRepliesAtItsVersion(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)
	route, err := createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	require.NoError(t, err)

	// act
	muted := *route
	muted.MutedReplies = []string{"comment", "push"}
	updated, updateErr := st.UpdateRoute(ctx, routingActor, muted, time.Now())
	_, staleErr := st.UpdateRoute(ctx, routingActor, muted, time.Now())
	entry, before, after := lastAudit(t, st)

	// assert
	require.NoError(t, updateErr)
	assert.Equal(t, []string{"comment", "push"}, updated.MutedReplies)
	assert.Equal(t, route.Version+1, updated.Version)
	assert.ErrorIs(t, staleErr, store.ErrVersionMismatch)
	assert.Equal(t, "route.update", entry.Action)
	assert.Equal(t, []any{}, before["muted_replies"])
	assert.Equal(t, []any{"comment", "push"}, after["muted_replies"])
}

// An event reaches the routes of every source whose scope holds its
// repository (SYSREQ-044, SYSREQ-046).
func TestDestinationsFollowTheScopes(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	l := newLayout(t, st)
	forge := l.paymentsRepo.ConnectionID
	everything := createSource(t, st, &store.Connection{ID: forge}, "everything", "instance", "")
	shared := *l.payments
	shared.Sources = append(shared.Sources, everything.ID)
	_, err := st.UpdateTeam(ctx, routingActor, shared, time.Now())
	require.NoError(t, err)
	_, err = createRoute(t, st, l.payments, l.paymentsRepo, l.paymentsRoom)
	require.NoError(t, err)
	_, err = createRoute(t, st, l.billing, l.billingRepo, l.billingRoom)
	require.NoError(t, err)
	_, err = st.CreateRoute(ctx, routingActor, store.Route{
		TeamID:       l.payments.ID,
		SourceID:     everything.ID,
		SinkID:       l.paymentsRoom.ID,
		MutedReplies: []string{"push"},
	}, time.Now())
	require.NoError(t, err)

	tests := []struct {
		repository string
		wantSinks  []int64
	}{
		{"payments/api", []int64{l.paymentsRoom.ID, l.paymentsRoom.ID}},
		{"billing/api", []int64{l.billingRoom.ID, l.paymentsRoom.ID}},
		{"billing/web", []int64{l.paymentsRoom.ID}},
		{"paymentsx/api", []int64{l.paymentsRoom.ID}},
	}
	for _, tc := range tests {
		t.Run(tc.repository, func(t *testing.T) {
			// act
			destinations, err := st.Destinations(ctx, forge, tc.repository)

			// assert
			require.NoError(t, err)
			sinks := make([]int64, 0, len(destinations))
			for _, d := range destinations {
				sinks = append(sinks, d.SinkID)
				assert.Equal(t, "c-"+map[int64]string{l.paymentsRoom.ID: "payments", l.billingRoom.ID: "billing"}[d.SinkID], d.ChannelID)
			}
			assert.ElementsMatch(t, tc.wantSinks, sinks, "a group holds what lies below it, and only that")
		})
	}
	other, err := st.Destinations(ctx, forge+1000, "payments/api")
	require.NoError(t, err)
	assert.Empty(t, other, "another connection's events reach none of these routes")
}
