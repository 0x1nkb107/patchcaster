package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/e2e/apiv1"
)

// routing is the stored configuration startRouted builds through the API,
// as SCEN-014, SCEN-015 and SCEN-018 do: a GitLab connection with the
// suite's signing token, a Mattermost connection, a source for the suite's
// project, a sink in the suite's channel, a team owning both, and a route
// between them.
type routing struct {
	admin    *browser
	provider *fakeProvider
	forge    *apiv1.Connection
	chat     *apiv1.Connection
	source   *apiv1.Source
	sink     *apiv1.Sink
	team     *apiv1.Team
	route    *apiv1.TeamRoute
}

// startRouted starts a service with cfg, configures it to route the suite's
// project into the suite's channel at mattermost, and points the service's
// fake GitLab at the new connection's endpoint. The returned config starts
// a later process of the same instance, which reads the same
// configuration.
func startRouted(t *testing.T, cfg serviceConfig, mattermost *fakeMattermost) (*service, *routing, serviceConfig) {
	t.Helper()
	ctx := context.Background()
	svc := startService(t, cfg)
	provider := newFakeProvider(t)
	r := &routing{
		admin:    setUp(t, svc, provider, setupRequest(bootstrap(t, svc), provider), ada),
		provider: provider,
	}
	admin := r.admin.api(t)

	var err error
	r.forge, err = admin.CreateConnection(ctx, &apiv1.ConnectionRequest{
		Kind:    apiv1.ConnectionKindGitlab,
		Name:    "gitlab",
		BaseURL: "https://gitlab.example",
	})
	require.NoError(t, err)
	_, err = admin.AddSigningToken(ctx, &apiv1.SigningTokenRequest{Label: "suite hook", Token: signingSecret},
		apiv1.AddSigningTokenParams{ID: r.forge.ID})
	require.NoError(t, err)
	r.chat = connectChat(t, r.admin, "mattermost", mattermost)
	r.source, err = admin.CreateSource(ctx, &apiv1.SourceRequest{
		ConnectionID: r.forge.ID,
		Name:         "application",
		Scope: apiv1.SourceScope{
			Kind: apiv1.SourceScopeKindRepository,
			Path: apiv1.NewOptString(projectPath),
		},
	})
	require.NoError(t, err)
	r.sink = r.addSink(t, r.chat, "suite channel", channelID)
	r.route = r.addRoute(t, r.sink, nil)

	cfg.forgeConnection = r.forge.ID
	svc.gitlab = gitlab{webhookURL: webhookURL(svc.publicAddr, r.forge.ID)}
	return svc, r, cfg
}

// connectChat creates a Mattermost connection named name to mattermost.
func connectChat(t *testing.T, admin *browser, name string, mattermost *fakeMattermost) *apiv1.Connection {
	t.Helper()
	chat, err := admin.api(t).CreateConnection(context.Background(), &apiv1.ConnectionRequest{
		Kind:       apiv1.ConnectionKindMattermost,
		Name:       name,
		BaseURL:    mattermost.url(),
		Credential: apiv1.NewOptString(botToken),
	})
	require.NoError(t, err)
	return chat
}

// addSink creates a sink in channel of chat and gives it to the routing's
// team, creating the team with the source at the first sink.
func (r *routing) addSink(t *testing.T, chat *apiv1.Connection, name, channel string) *apiv1.Sink {
	t.Helper()
	ctx := context.Background()
	admin := r.admin.api(t)
	sink, err := admin.CreateSink(ctx, &apiv1.SinkRequest{
		ConnectionID: chat.ID,
		Name:         name,
		ChannelID:    channel,
	})
	require.NoError(t, err)
	if r.team == nil {
		r.team, err = admin.CreateTeam(ctx, &apiv1.TeamRequest{Name: "Application"})
		require.NoError(t, err)
	}
	sinks := append(append([]int64{}, r.team.Sinks...), sink.ID)
	updated, err := admin.UpdateTeam(ctx, &apiv1.TeamUpdate{
		Name:      r.team.Name,
		Operators: []int64{},
		Sources:   []int64{r.source.ID},
		Sinks:     sinks,
	}, apiv1.UpdateTeamParams{ID: r.team.ID, IfMatch: r.team.Etag})
	require.NoError(t, err)
	r.team = &updated.Response
	return sink
}

// addRoute routes the source into sink, muting the given replies.
func (r *routing) addRoute(t *testing.T, sink *apiv1.Sink, muted []apiv1.ReplyKind) *apiv1.TeamRoute {
	t.Helper()
	route, err := r.admin.api(t).CreateRoute(context.Background(), &apiv1.RouteRequest{
		TeamID:       r.team.ID,
		SourceID:     r.source.ID,
		SinkID:       sink.ID,
		MutedReplies: muted,
	})
	require.NoError(t, err)
	return route
}
