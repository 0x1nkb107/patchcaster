package e2e

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/e2e/apiv1"
)

// rootTemplate is a root template that differs from the built-in one.
const rootTemplate = "**!{{.MergeRequest.Number}} {{.MergeRequest.Title}}** by {{.MergeRequest.Author}}"

// A Routing Manager gives their team's route its own root template and
// takes it back again (SCEN-021, SYSREQ-049). Each change makes a new
// version of the route and appears in the audit log (SCEN-016).
func TestAManagerGivesTheirRouteATemplate(t *testing.T) {
	// arrange
	ctx := context.Background()
	s := setUpTeams(t)
	lead := s.manager.api(t)
	route, err := createRoute(t, lead, s.payments, s.paymentsRepo, s.paymentsRoom)
	require.NoError(t, err)
	root := apiv1.SetRouteTemplateParams{ID: route.ID, Kind: apiv1.MessageKindRoot, IfMatch: route.Etag}

	// act
	set, setErr := lead.SetRouteTemplate(ctx, &apiv1.RouteTemplateUpdate{Version: 1, Text: rootTemplate}, root)
	require.NoError(t, setErr)
	read, readErr := lead.GetRouteTemplate(ctx, apiv1.GetRouteTemplateParams{ID: route.ID, Kind: apiv1.MessageKindRoot})
	withTemplate, withErr := lead.GetRoute(ctx, apiv1.GetRouteParams{ID: route.ID})
	_, staleErr := lead.SetRouteTemplate(ctx, &apiv1.RouteTemplateUpdate{Version: 1, Text: rootTemplate}, root)
	deleted, deleteErr := lead.DeleteRouteTemplate(ctx, apiv1.DeleteRouteTemplateParams{ID: route.ID, Kind: apiv1.MessageKindRoot, IfMatch: set.Response.Etag})
	_, goneErr := lead.GetRouteTemplate(ctx, apiv1.GetRouteTemplateParams{ID: route.ID, Kind: apiv1.MessageKindRoot})
	without, withoutErr := lead.GetRoute(ctx, apiv1.GetRouteParams{ID: route.ID})

	// assert
	assert.Equal(t, apiv1.TemplateText(rootTemplate), set.Response.Text)
	assert.NotEqual(t, route.Etag, set.Response.Etag, "a template makes a new version of the route")
	require.NoError(t, readErr)
	assert.Equal(t, apiv1.TemplateVersion(1), read.Response.Version)
	assert.Equal(t, apiv1.TemplateText(rootTemplate), read.Response.Text)
	require.NoError(t, withErr)
	assert.Equal(t, []apiv1.MessageKind{apiv1.MessageKindRoot}, withTemplate.Response.CustomTemplates)
	assert.Equal(t, set.Response.Etag, withTemplate.Response.Etag)
	requireProblem(t, staleErr, http.StatusPreconditionFailed)
	require.NoError(t, deleteErr)
	requireProblem(t, goneErr, http.StatusNotFound)
	require.NoError(t, withoutErr)
	assert.Empty(t, without.Response.CustomTemplates)
	assert.Equal(t, without.Response.Etag, deleted.ETag.Or(""), "the removal answers with the route's new version")
	assert.Subset(t, auditActions(t, s.admin), []string{"route.template_set", "route.template_delete"})
}

// A template that fails a sample is refused with its line and cause, and
// nothing changes (ADR-038).
func TestARefusedTemplateChangesNothing(t *testing.T) {
	// arrange
	ctx := context.Background()
	s := setUpTeams(t)
	lead := s.manager.api(t)
	route, err := createRoute(t, lead, s.payments, s.paymentsRepo, s.paymentsRoom)
	require.NoError(t, err)

	// act
	_, refusedErr := lead.SetRouteTemplate(ctx,
		&apiv1.RouteTemplateUpdate{Version: 1, Text: "fine\n{{printf \"%s\" .Event.Actor}}"},
		apiv1.SetRouteTemplateParams{ID: route.ID, Kind: apiv1.MessageKindComment, IfMatch: route.Etag})
	after, afterErr := lead.GetRoute(ctx, apiv1.GetRouteParams{ID: route.ID})

	// assert
	refused := requireProblem(t, refusedErr, http.StatusUnprocessableEntity)
	require.Len(t, refused.Errors, 1)
	assert.Equal(t, "text", refused.Errors[0].Path)
	assert.Equal(t, []string{"line 2: printf is not a template function"}, refused.Errors[0].Messages)
	require.NoError(t, afterErr)
	assert.Equal(t, route.Etag, after.Response.Etag)
	assert.Empty(t, after.Response.CustomTemplates)
}

// Templates stay inside the team, like the rest of its routing
// (SYSREQ-039).
func TestTemplatesStayInsideTheTeam(t *testing.T) {
	// arrange
	ctx := context.Background()
	s := setUpTeams(t)
	lead := s.manager.api(t)
	billingRoute, err := createRoute(t, s.admin.api(t), s.billing, s.billingRepo, s.billingRoom)
	require.NoError(t, err)
	billingRoot := apiv1.SetRouteTemplateParams{ID: billingRoute.ID, Kind: apiv1.MessageKindRoot, IfMatch: billingRoute.Etag}
	_, err = s.admin.api(t).SetRouteTemplate(ctx, &apiv1.RouteTemplateUpdate{Version: 1, Text: rootTemplate}, billingRoot)
	require.NoError(t, err)
	user := logInAs(t, s.svc, s.provider, bob).api(t)

	tests := []struct {
		name string
		call func() error
	}{
		{"another team's template read", func() error {
			_, err := lead.GetRouteTemplate(ctx, apiv1.GetRouteTemplateParams{ID: billingRoute.ID, Kind: apiv1.MessageKindRoot})
			return err
		}},
		{"another team's template set", func() error {
			_, err := lead.SetRouteTemplate(ctx, &apiv1.RouteTemplateUpdate{Version: 1, Text: rootTemplate}, billingRoot)
			return err
		}},
		{"another team's template removed", func() error {
			_, err := lead.DeleteRouteTemplate(ctx, apiv1.DeleteRouteTemplateParams{ID: billingRoute.ID, Kind: apiv1.MessageKindRoot, IfMatch: billingRoute.Etag})
			return err
		}},
		{"a template set by a user", func() error {
			_, err := user.SetRouteTemplate(ctx, &apiv1.RouteTemplateUpdate{Version: 1, Text: rootTemplate}, billingRoot)
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requireProblem(t, tc.call(), http.StatusForbidden)
		})
	}
}

// Anyone logged in previews a draft for each platform, without storing
// it (SCEN-021).
func TestADraftIsPreviewedForMattermost(t *testing.T) {
	// arrange
	ctx := context.Background()
	s := setUpTeams(t)
	user := logInAs(t, s.svc, s.provider, bob).api(t)

	// act
	preview, err := user.PreviewTemplate(ctx, &apiv1.TemplateDraft{Kind: apiv1.MessageKindRoot, Version: 1, Text: rootTemplate})

	// assert
	require.NoError(t, err)
	require.NotEmpty(t, preview.Samples)
	assert.Equal(t, "an open merge request", preview.Samples[0].Description)
	assert.Equal(t, "**!12 Add flux capacitor** by ada", preview.Samples[0].Mattermost)
}
