package e2e

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/e2e/apiv1"
)

// teamSetup is SCEN-015's result: the team "Payments" with carol as its
// operator, owning a group source and a sink in the suite's channel. A
// second team, "Billing", owns its own source and sink.
type teamSetup struct {
	platforms
	operator     *browser
	payments     *apiv1.Team
	billing      *apiv1.Team
	paymentsRepo *apiv1.Source
	billingRepo  *apiv1.Source
	paymentsRoom *apiv1.Sink
	billingRoom  *apiv1.Sink
}

// setUpTeams builds the teamSetup through the API, as an administrator.
func setUpTeams(t *testing.T) teamSetup {
	t.Helper()
	p := startWithPlatforms(t, serviceConfig{})
	forge := p.connectGitLab(t)
	chat := p.connectMattermost(t)
	s := teamSetup{platforms: p}
	s.operator = logInAs(t, p.svc, p.provider, carol)
	carolRead := personNamed(t, p.admin, "carol")
	s.paymentsRepo = p.createSource(t, forge, "payments", apiv1.SourceScopeKindGroup, "payments")
	s.billingRepo = p.createSource(t, forge, "billing", apiv1.SourceScopeKindRepository, "billing/api")
	s.paymentsRoom = p.createSink(t, chat, "payments room", channelID)
	s.billingRoom = p.createSink(t, chat, "billing room", otherChannelID)
	s.payments = p.createTeam(t, "Payments", []int64{carolRead.ID}, s.paymentsRepo, s.paymentsRoom)
	s.billing = p.createTeam(t, "Billing", nil, s.billingRepo, s.billingRoom)
	return s
}

func (p platforms) createSource(t *testing.T, forge *apiv1.Connection, name string, kind apiv1.SourceScopeKind, path string) *apiv1.Source {
	t.Helper()
	source, err := p.admin.api(t).CreateSource(context.Background(), &apiv1.SourceRequest{
		ConnectionID: forge.ID,
		Name:         name,
		Scope: apiv1.SourceScope{
			Kind: kind,
			Path: apiv1.NewOptString(path),
		},
	})
	require.NoError(t, err)
	return source
}

func (p platforms) createSink(t *testing.T, chat *apiv1.Connection, name, channel string) *apiv1.Sink {
	t.Helper()
	sink, err := p.admin.api(t).CreateSink(context.Background(), &apiv1.SinkRequest{
		ConnectionID: chat.ID,
		Name:         name,
		ChannelID:    channel,
	})
	require.NoError(t, err)
	return sink
}

// createTeam creates a team and gives it operators, a source and a sink.
func (p platforms) createTeam(t *testing.T, name string, operators []int64, source *apiv1.Source, sink *apiv1.Sink) *apiv1.Team {
	t.Helper()
	ctx := context.Background()
	created, err := p.admin.api(t).CreateTeam(ctx, &apiv1.TeamRequest{Name: name})
	require.NoError(t, err)
	if operators == nil {
		operators = []int64{}
	}
	updated, err := p.admin.api(t).UpdateTeam(ctx, &apiv1.TeamUpdate{
		Name:      name,
		Operators: operators,
		Sources:   []int64{source.ID},
		Sinks:     []int64{sink.ID},
	}, apiv1.UpdateTeamParams{ID: created.ID, IfMatch: created.Etag})
	require.NoError(t, err)
	return &updated.Response
}

// createRoute routes source into sink for team, as the browser's person.
func createRoute(t *testing.T, b *apiv1.Client, team *apiv1.Team, source *apiv1.Source, sink *apiv1.Sink) (*apiv1.TeamRoute, error) {
	t.Helper()
	return b.CreateRoute(context.Background(), &apiv1.RouteRequest{
		TeamID:   team.ID,
		SourceID: source.ID,
		SinkID:   sink.ID,
	})
}

// An administrator sets up a team: a source, a sink the bot can post
// into, and an operator (SCEN-015).
func TestAnAdministratorSetsUpATeam(t *testing.T) {
	// arrange
	ctx := context.Background()
	s := setUpTeams(t)

	// act
	_, lobbyErr := s.admin.api(t).CreateSink(ctx, &apiv1.SinkRequest{
		ConnectionID: s.paymentsRoom.ConnectionID,
		Name:         "lobby",
		ChannelID:    lobbyChannelID,
	})
	_, nowhereErr := s.admin.api(t).CreateSink(ctx, &apiv1.SinkRequest{
		ConnectionID: s.paymentsRoom.ConnectionID,
		Name:         "nowhere",
		ChannelID:    "chan-nowhere",
	})
	_, archivedErr := s.admin.api(t).CreateSink(ctx, &apiv1.SinkRequest{
		ConnectionID: s.paymentsRoom.ConnectionID,
		Name:         "archive",
		ChannelID:    archivedChannelID,
	})
	team, teamErr := s.operator.api(t).GetTeam(ctx, apiv1.GetTeamParams{ID: s.payments.ID})

	// assert
	assert.Equal(t, "Channel "+channelID, s.paymentsRoom.ChannelName, "the sink shows the channel's name")
	lobby := requireProblem(t, lobbyErr, http.StatusUnprocessableEntity)
	require.Len(t, lobby.Errors, 1)
	assert.Equal(t, "channel_id", lobby.Errors[0].Path)
	assert.Equal(t, []string{"the bot is not a member of the channel: add it to the channel"}, lobby.Errors[0].Messages)
	requireProblem(t, nowhereErr, http.StatusUnprocessableEntity)
	archived := requireProblem(t, archivedErr, http.StatusUnprocessableEntity)
	require.Len(t, archived.Errors, 1)
	assert.Equal(t, []string{"the channel is archived"}, archived.Errors[0].Messages)
	require.NoError(t, teamErr, "the operator sees their team")
	assert.Equal(t, []int64{s.paymentsRepo.ID}, team.Response.Sources)
	assert.Subset(t, auditActions(t, s.admin), []string{"team.create", "team.update", "source.create", "sink.create"})
}

// An operator routes their team's source into its sink, and mutes a reply
// kind, without an administrator (SCEN-018).
func TestAnOperatorRoutesTheirTeamsSource(t *testing.T) {
	// arrange
	ctx := context.Background()
	s := setUpTeams(t)
	lead := s.operator.api(t)

	// act
	sources, sourcesErr := lead.ListSources(ctx, apiv1.ListSourcesParams{})
	sinks, sinksErr := lead.ListSinks(ctx, apiv1.ListSinksParams{})
	route, routeErr := createRoute(t, lead, s.payments, s.paymentsRepo, s.paymentsRoom)
	require.NoError(t, routeErr)
	muted, muteErr := lead.UpdateRoute(ctx,
		&apiv1.RouteUpdate{MutedReplies: []apiv1.ReplyKind{apiv1.ReplyKindComment}},
		apiv1.UpdateRouteParams{ID: route.ID, IfMatch: route.Etag})

	// assert
	require.NoError(t, sourcesErr)
	require.Len(t, sources.Items, 1, "an operator sees only their team's sources")
	assert.Equal(t, s.paymentsRepo.ID, sources.Items[0].ID)
	require.NoError(t, sinksErr)
	require.Len(t, sinks.Items, 1)
	assert.Empty(t, route.MutedReplies, "the default filter lets every reply through")
	require.NoError(t, muteErr)
	assert.Equal(t, []apiv1.ReplyKind{apiv1.ReplyKindComment}, muted.Response.MutedReplies)
	assert.Subset(t, auditActions(t, s.admin), []string{"route.create", "route.update"})
}

// An operator neither reads nor changes another team's routing, and a user
// without a team routes nothing (SYSREQ-039).
func TestRoutingStaysInsideTheTeam(t *testing.T) {
	// arrange
	ctx := context.Background()
	s := setUpTeams(t)
	lead := s.operator.api(t)
	billingRoute, err := createRoute(t, s.admin.api(t), s.billing, s.billingRepo, s.billingRoom)
	require.NoError(t, err)
	user := logInAs(t, s.svc, s.provider, bob).api(t)

	tests := []struct {
		name string
		call func() error
	}{
		{"another team's source in a route", func() error {
			_, err := createRoute(t, lead, s.payments, s.billingRepo, s.paymentsRoom)
			return err
		}},
		{"another team's sink in a route", func() error {
			_, err := createRoute(t, lead, s.payments, s.paymentsRepo, s.billingRoom)
			return err
		}},
		{"a route for another team", func() error {
			_, err := createRoute(t, lead, s.billing, s.billingRepo, s.billingRoom)
			return err
		}},
		{"another team", func() error {
			_, err := lead.GetTeam(ctx, apiv1.GetTeamParams{ID: s.billing.ID})
			return err
		}},
		{"another team's route", func() error {
			_, err := lead.GetRoute(ctx, apiv1.GetRouteParams{ID: billingRoute.ID})
			return err
		}},
		{"another team's routes listed", func() error {
			_, err := lead.ListRoutes(ctx, apiv1.ListRoutesParams{TeamID: apiv1.NewOptInt64(s.billing.ID)})
			return err
		}},
		{"another team's source read", func() error {
			_, err := lead.GetSource(ctx, apiv1.GetSourceParams{ID: s.billingRepo.ID})
			return err
		}},
		{"a source created by an operator", func() error {
			_, err := lead.CreateSource(ctx, &apiv1.SourceRequest{
				ConnectionID: s.paymentsRepo.ConnectionID,
				Name:         "sneaky",
				Scope:        apiv1.SourceScope{Kind: apiv1.SourceScopeKindInstance},
			})
			return err
		}},
		{"a route created by a user", func() error {
			_, err := createRoute(t, user, s.payments, s.paymentsRepo, s.paymentsRoom)
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			err := tc.call()

			// assert
			requireProblem(t, err, http.StatusForbidden)
		})
	}
}

// What a route uses stays until the route goes.
func TestARouteKeepsItsSourceAndSink(t *testing.T) {
	// arrange
	ctx := context.Background()
	s := setUpTeams(t)
	route, err := createRoute(t, s.admin.api(t), s.payments, s.paymentsRepo, s.paymentsRoom)
	require.NoError(t, err)
	team, err := s.admin.api(t).GetTeam(ctx, apiv1.GetTeamParams{ID: s.payments.ID})
	require.NoError(t, err)

	// act
	_, withoutSourceErr := s.admin.api(t).UpdateTeam(ctx, &apiv1.TeamUpdate{
		Name:      team.Response.Name,
		Operators: team.Response.Operators,
		Sources:   []int64{},
		Sinks:     team.Response.Sinks,
	}, apiv1.UpdateTeamParams{ID: team.Response.ID, IfMatch: team.Response.Etag})
	deleteSourceErr := s.admin.api(t).DeleteSource(ctx, apiv1.DeleteSourceParams{ID: s.paymentsRepo.ID, IfMatch: s.paymentsRepo.Etag})
	deleteTeamErr := s.admin.api(t).DeleteTeam(ctx, apiv1.DeleteTeamParams{ID: team.Response.ID, IfMatch: team.Response.Etag})
	deleteRouteErr := s.admin.api(t).DeleteRoute(ctx, apiv1.DeleteRouteParams{ID: route.ID, IfMatch: route.Etag})
	afterErr := s.admin.api(t).DeleteSource(ctx, apiv1.DeleteSourceParams{ID: s.paymentsRepo.ID, IfMatch: s.paymentsRepo.Etag})

	// assert
	withoutSource := requireProblem(t, withoutSourceErr, http.StatusConflict)
	require.Len(t, withoutSource.Errors, 1)
	assert.Equal(t, "sources", withoutSource.Errors[0].Path)
	requireProblem(t, deleteSourceErr, http.StatusConflict)
	requireProblem(t, deleteTeamErr, http.StatusConflict)
	require.NoError(t, deleteRouteErr)
	assert.NoError(t, afterErr, "without its route the source goes")
}

// Automation manages its team's routes through an operator's token
// (SCEN-022).
func TestAnOperatorsTokenManagesTheTeamsRoutes(t *testing.T) {
	// arrange
	s := setUpTeams(t)
	ci := automation(t, s.svc, createToken(t, s.operator, "routes from git").Secret)

	// act
	_, ownErr := createRoute(t, ci, s.payments, s.paymentsRepo, s.paymentsRoom)
	_, otherErr := createRoute(t, ci, s.billing, s.billingRepo, s.billingRoom)
	audit, auditErr := s.admin.api(t).ListAudit(context.Background(), apiv1.ListAuditParams{})

	// assert
	require.NoError(t, ownErr)
	requireProblem(t, otherErr, http.StatusForbidden)
	require.NoError(t, auditErr)
	assert.Equal(t, "route.create", audit.Items[0].Action)
	assert.Equal(t, "routes from git", audit.Items[0].ActorToken.Or(""), "the change names the token and its owner")
	assert.Equal(t, "Carol", audit.Items[0].Actor)
}
