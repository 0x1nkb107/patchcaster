package api

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/markdown"
	"github.com/patchcaster-app/patchcaster/internal/message"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// A route's own templates replace the built-in ones, one kind of message at
// a time (SYSREQ-049, ADR-038). Whoever may change a route's muted replies
// may change its templates.

// customTemplates returns the kinds a route has templates of its own for,
// in the order of message.Kinds, the root first.
func customTemplates(kinds []string) []v1gen.MessageKind {
	custom := []v1gen.MessageKind{}
	for _, kind := range message.Kinds {
		if slices.Contains(kinds, string(kind)) {
			custom = append(custom, v1gen.MessageKind(kind))
		}
	}
	return custom
}

// templateProblem turns a refusal of the template engine into a 422 that
// names the template's text, with the line and the cause.
func templateProblem(err error) error {
	var refusal *message.Error
	if !errors.As(err, &refusal) {
		return err
	}
	return fieldProblem(http.StatusUnprocessableEntity, "the template is refused", "text", refusal.Error())
}

// parsedTemplate parses a template of a known version of the data model.
func parsedTemplate(kind v1gen.MessageKind, version v1gen.TemplateVersion, text v1gen.TemplateText) (*message.Template, error) {
	if !message.Version(version).Known() {
		return nil, fieldProblem(http.StatusUnprocessableEntity, "the template is refused", "version", "names no version of the data model")
	}
	template, err := message.Parse(message.Kind(kind), message.Version(version), string(text))
	if err != nil {
		return nil, templateProblem(err)
	}
	return template, nil
}

func routeTemplateHeaders(t *store.RouteTemplate, routeVersion int64) *v1gen.RouteTemplateHeaders {
	etag := etagOf(routeVersion)
	return &v1gen.RouteTemplateHeaders{
		ETag: v1gen.NewOptString(etag),
		Response: v1gen.RouteTemplate{
			Kind:    v1gen.MessageKind(t.Kind),
			Version: v1gen.TemplateVersion(t.DataVersion),
			Text:    v1gen.TemplateText(t.Text),
			Etag:    etag,
		},
	}
}

// GetRouteTemplate reads a route's own template for one kind of message,
// for whoever may see the route.
func (s *Service) GetRouteTemplate(ctx context.Context, params v1gen.GetRouteTemplateParams) (*v1gen.RouteTemplateHeaders, error) {
	route, err := s.store.Route(ctx, params.ID)
	if err != nil {
		return nil, storeProblem(err)
	}
	if err := s.mayReadTeam(ctx, route.TeamID); err != nil {
		return nil, err
	}
	template, err := s.store.RouteTemplate(ctx, route.ID, string(params.Kind))
	if err != nil {
		return nil, storeProblem(err)
	}
	return routeTemplateHeaders(template, route.Version), nil
}

// SetRouteTemplate gives a route its own template for one kind of message,
// for the team's managers and administrators. A template that fails a
// sample is refused (ADR-038).
func (s *Service) SetRouteTemplate(ctx context.Context, req *v1gen.RouteTemplateUpdate, params v1gen.SetRouteTemplateParams) (*v1gen.RouteTemplateHeaders, error) {
	current, err := s.manageableRoute(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return nil, err
	}
	parsed, err := parsedTemplate(params.Kind, req.Version, req.Text)
	if err != nil {
		return nil, err
	}
	if err := message.Check(parsed); err != nil {
		return nil, templateProblem(err)
	}
	template := store.RouteTemplate{
		RouteID:     current.ID,
		Kind:        string(params.Kind),
		DataVersion: int(req.Version),
		Text:        string(req.Text),
	}
	updated, err := s.store.SetRouteTemplate(ctx, actorOf(ctx), template, version, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	return routeTemplateHeaders(&template, updated.Version), nil
}

// DeleteRouteTemplate returns a route to the built-in template for one kind
// of message, for the team's managers and administrators.
func (s *Service) DeleteRouteTemplate(ctx context.Context, params v1gen.DeleteRouteTemplateParams) (*v1gen.DeleteRouteTemplateNoContent, error) {
	current, err := s.manageableRoute(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return nil, err
	}
	updated, err := s.store.DeleteRouteTemplate(ctx, actorOf(ctx), current.ID, string(params.Kind), version, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	return &v1gen.DeleteRouteTemplateNoContent{ETag: v1gen.NewOptString(etagOf(updated.Version))}, nil
}

// PreviewTemplate renders a draft template against the sample merge
// requests, for each chat platform, without storing it (SCEN-021). It
// reads no configuration, so every person who is logged in may use it.
func (s *Service) PreviewTemplate(_ context.Context, req *v1gen.TemplateDraft) (*v1gen.TemplatePreview, error) {
	template, err := parsedTemplate(req.Kind, req.Version, req.Text)
	if err != nil {
		return nil, err
	}
	renderings, err := message.Preview(template)
	if err != nil {
		return nil, templateProblem(err)
	}
	samples := make([]v1gen.TemplateSample, 0, len(renderings))
	for _, rendering := range renderings {
		samples = append(samples, v1gen.TemplateSample{
			Description: rendering.Description,
			Mattermost:  markdown.Mattermost(markdown.Parse(rendering.Markdown)),
		})
	}
	return &v1gen.TemplatePreview{Samples: samples}, nil
}
