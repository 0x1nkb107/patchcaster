package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/e2e/apiv1"
)

// allTriggers are the triggers of a hook that takes every event.
const allTriggers = "merge_requests,note,pipeline,push"

// connectHookedForge creates a GitLab connection to api with API access,
// so that PatchCaster registers its hooks.
func connectHookedForge(t *testing.T, admin *apiv1.Client, api *fakeGitLabAPI) *apiv1.Connection {
	t.Helper()
	forge, err := admin.CreateConnection(context.Background(), &apiv1.ConnectionRequest{
		Kind:       apiv1.ConnectionKindGitlab,
		Name:       "hooked gitlab",
		BaseURL:    api.url(),
		Credential: apiv1.NewOptString(gitlabToken),
	})
	require.NoError(t, err)
	return forge
}

// createSource creates a source of forge with the given scope.
func createSource(t *testing.T, admin *apiv1.Client, forge *apiv1.Connection, name string, kind apiv1.SourceScopeKind, path string) *apiv1.Source {
	t.Helper()
	scope := apiv1.SourceScope{Kind: kind}
	if path != "" {
		scope.Path = apiv1.NewOptString(path)
	}
	source, err := admin.CreateSource(context.Background(), &apiv1.SourceRequest{
		ConnectionID: forge.ID,
		Name:         name,
		Scope:        scope,
	})
	require.NoError(t, err)
	return source
}

func deleteSource(t *testing.T, admin *apiv1.Client, source *apiv1.Source) {
	t.Helper()
	require.NoError(t, admin.DeleteSource(context.Background(), apiv1.DeleteSourceParams{ID: source.ID, IfMatch: source.Etag}))
}

// expectLayout waits until the fake GitLab holds exactly the hooks
// described.
func expectLayout(t *testing.T, api *fakeGitLabAPI, want ...string) {
	t.Helper()
	eventually(t, 10*time.Second, "the hooks "+strings.Join(want, "; "), func() bool {
		got := api.layout()
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	})
}

// hookOf reads how the events of a source reach PatchCaster.
func hookOf(t *testing.T, admin *apiv1.Client, source *apiv1.Source) apiv1.SourceHook {
	t.Helper()
	read, err := admin.GetSource(context.Background(), apiv1.GetSourceParams{ID: source.ID})
	require.NoError(t, err)
	return read.Response.Hook
}

// PatchCaster registers one hook for every scope no broader one covers. A
// broader hook replaces the narrower ones, and removing it brings them
// back. An instance source takes merge requests and pushes through a
// system hook, and the narrower hooks keep comments and pipelines
// (SYSREQ-044).
func TestHooksFollowTheSourceScopes(t *testing.T) {
	// arrange
	mattermost := newFakeMattermost(t)
	freshDatabase(t)
	_, r, _ := startRouted(t, serviceConfig{}, mattermost)
	api := newFakeGitLabAPI(t)
	admin := r.admin.api(t)
	forge := connectHookedForge(t, admin, api)

	// act & assert: a repository gets a hook of its own.
	repo := createSource(t, admin, forge, "hooked application", apiv1.SourceScopeKindRepository, projectPath)
	expectLayout(t, api, "projects/group/application: "+allTriggers)
	registered := api.hooksAt("projects/group/application")[0]
	assert.Equal(t, forge.WebhookURL.Or(""), registered.URL)
	assert.Equal(t, "PatchCaster", registered.Name)
	assert.True(t, strings.HasPrefix(registered.SigningToken, "whsec_"), "PatchCaster generates the signing token")

	// act & assert: its group takes over.
	group := createSource(t, admin, forge, "hooked group", apiv1.SourceScopeKindGroup, "group")
	expectLayout(t, api, "groups/group: "+allTriggers)
	repoHook := hookOf(t, admin, repo)
	assert.Equal(t, apiv1.SourceHookStateCovered, repoHook.State)
	assert.Equal(t, "group", repoHook.CoveredBy.Value.Path.Or(""))
	eventually(t, 10*time.Second, "the group's hook to show as registered", func() bool {
		return hookOf(t, admin, group).State == apiv1.SourceHookStateRegistered
	})

	// act & assert: the instance takes merge requests and pushes.
	instance := createSource(t, admin, forge, "hooked instance", apiv1.SourceScopeKindInstance, "")
	expectLayout(t, api, "groups/group: note,pipeline", "system: merge_requests,push")

	// act & assert: without the instance, the group takes everything again.
	deleteSource(t, admin, instance)
	expectLayout(t, api, "groups/group: "+allTriggers)

	// act & assert: without the group, the repository gets its hook back.
	deleteSource(t, admin, group)
	expectLayout(t, api, "projects/group/application: "+allTriggers)

	// act & assert: without sources, no hook stays, and the connection can go.
	deleteSource(t, admin, repo)
	expectLayout(t, api)
	eventually(t, 10*time.Second, "the connection to go", func() bool {
		read, err := admin.GetConnection(context.Background(), apiv1.GetConnectionParams{ID: forge.ID})
		require.NoError(t, err)
		return admin.DeleteConnection(context.Background(), apiv1.DeleteConnectionParams{ID: forge.ID, IfMatch: read.Response.Etag}) == nil
	})
}

// One delivery through the registered group hook reaches each matching
// source exactly once: the group's and the repository's, routed to a sink
// each (SYSREQ-044).
func TestADeliveryThroughARegisteredHookReachesEachMatchingSourceOnce(t *testing.T) {
	// arrange
	mattermost := newFakeMattermost(t)
	freshDatabase(t)
	_, r, _ := startRouted(t, serviceConfig{}, mattermost)
	api := newFakeGitLabAPI(t)
	admin := r.admin.api(t)
	ctx := context.Background()
	forge := connectHookedForge(t, admin, api)
	group := createSource(t, admin, forge, "hooked group", apiv1.SourceScopeKindGroup, "group")
	repo := createSource(t, admin, forge, "hooked application", apiv1.SourceScopeKindRepository, projectPath)
	other, err := admin.CreateSink(ctx, &apiv1.SinkRequest{ConnectionID: r.chat.ID, Name: "other channel", ChannelID: otherChannelID})
	require.NoError(t, err)
	team, err := admin.CreateTeam(ctx, &apiv1.TeamRequest{Name: "Hooked"})
	require.NoError(t, err)
	_, err = admin.UpdateTeam(ctx, &apiv1.TeamUpdate{
		Name:      team.Name,
		Operators: []int64{},
		Sources:   []int64{group.ID, repo.ID},
		Sinks:     []int64{r.sink.ID, other.ID},
	}, apiv1.UpdateTeamParams{ID: team.ID, IfMatch: team.Etag})
	require.NoError(t, err)
	for _, route := range []apiv1.RouteRequest{
		{TeamID: team.ID, SourceID: group.ID, SinkID: r.sink.ID},
		{TeamID: team.ID, SourceID: repo.ID, SinkID: other.ID},
	} {
		_, err := admin.CreateRoute(ctx, &route)
		require.NoError(t, err)
	}
	expectLayout(t, api, "groups/group: "+allTriggers)
	token := api.hooksAt("groups/group")[0].SigningToken
	hook := gitlab{webhookURL: forge.WebhookURL.Or("")}

	// act
	status := hook.deliverSigned(t, newDeliveryID(t), mergeRequestOpened(t, firstMR), token)

	// assert
	require.Equal(t, http.StatusOK, status, "the generated token verifies the hook's delivery")
	eventually(t, 10*time.Second, "a root in each routed channel", func() bool {
		return len(mattermost.acceptedPosts()) == 2
	})
	assert.Len(t, postsIn(mattermost, channelID), 1, "the group's route posts once")
	assert.Len(t, postsIn(mattermost, otherChannelID), 1, "the repository's route posts once")
}

// A GitLab without group hooks refuses one, and the source shows GitLab's
// answer (owner decision 2026-10-18).
func TestARefusedGroupHookShowsGitLabsAnswer(t *testing.T) {
	// arrange
	mattermost := newFakeMattermost(t)
	freshDatabase(t)
	_, r, _ := startRouted(t, serviceConfig{}, mattermost)
	api := newFakeGitLabAPI(t)
	api.refuseGroupHooks()
	admin := r.admin.api(t)
	forge := connectHookedForge(t, admin, api)

	// act
	group := createSource(t, admin, forge, "hooked group", apiv1.SourceScopeKindGroup, "group")

	// assert
	eventually(t, 10*time.Second, "the refusal on the source", func() bool {
		return hookOf(t, admin, group).State == apiv1.SourceHookStateFailed
	})
	assert.Contains(t, hookOf(t, admin, group).LastError.Or(""), "404 Not Found")
	assert.Empty(t, api.layout())
}

// Without API access, a source's hook is registered by hand.
func TestASourceWithoutAPIAccessIsManual(t *testing.T) {
	// arrange
	mattermost := newFakeMattermost(t)
	freshDatabase(t)

	// act
	_, r, _ := startRouted(t, serviceConfig{}, mattermost)

	// assert
	assert.Equal(t, apiv1.SourceHookStateManual, hookOf(t, r.admin.api(t), r.source).State)
}
