package hooks

import (
	"context"
	"fmt"
	"net/url"

	"github.com/patchcaster-app/patchcaster/internal/gitlab"
	"github.com/patchcaster-app/patchcaster/internal/platform"
	"github.com/patchcaster-app/patchcaster/internal/seal"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// ForgeRegistrars opens the registrar of a forge connection with its
// stored credential, for each forge PatchCaster registers hooks at.
type ForgeRegistrars struct {
	Box *seal.Box
}

// Registrar returns the registrar of connection, which must hold a
// credential.
func (f ForgeRegistrars) Registrar(_ context.Context, connection *store.Connection) (Registrar, error) {
	if platform.Kind(connection.Kind) != platform.GitLab {
		return nil, fmt.Errorf("connection %d of kind %q registers no hooks", connection.ID, connection.Kind)
	}
	credential, err := f.Box.Open(connection.Credential, store.ConnectionCredentialBinding(connection.ID))
	if err != nil {
		return nil, fmt.Errorf("open credential of connection %d: %w", connection.ID, err)
	}
	baseURL, err := url.Parse(connection.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL of connection %d: %w", connection.ID, err)
	}
	return gitlabRegistrar{client: gitlab.New(baseURL, string(credential))}, nil
}

// gitlabRegistrar registers hooks through the GitLab API: a system hook
// for the instance, and group and project hooks for the narrower scopes.
type gitlabRegistrar struct {
	client *gitlab.Client
}

func gitlabScope(scope store.HookScope) gitlab.HookScope {
	switch scope.Kind {
	case scopeInstance:
		return gitlab.HookScope{Kind: gitlab.SystemHook}
	case scopeGroup:
		return gitlab.HookScope{Kind: gitlab.GroupHook, Path: scope.Path}
	default:
		return gitlab.HookScope{Kind: gitlab.ProjectHook, Path: scope.Path}
	}
}

func gitlabSpec(spec Spec) gitlab.HookSpec {
	return gitlab.HookSpec{
		URL:         spec.URL,
		Name:        spec.Name,
		Description: spec.Description,
		Events: gitlab.HookEvents{
			MergeRequests: spec.Events.MergeRequests,
			Pushes:        spec.Events.Pushes,
			Comments:      spec.Events.Comments,
			Pipelines:     spec.Events.Pipelines,
		},
		SigningToken: spec.SigningToken,
	}
}

// gone turns GitLab's answer for a missing hook into ErrHookGone.
func gone(err error) error {
	if gitlab.IsNotFound(err) {
		return fmt.Errorf("%w: %w", ErrHookGone, err)
	}
	return err
}

func (g gitlabRegistrar) Hooks(ctx context.Context, scope store.HookScope) ([]ForgeHook, error) {
	listed, err := g.client.Hooks(ctx, gitlabScope(scope))
	if err != nil {
		return nil, err
	}
	hooks := make([]ForgeHook, 0, len(listed))
	for _, h := range listed {
		hooks = append(hooks, ForgeHook{ID: h.ID, URL: h.URL, Name: h.Name})
	}
	return hooks, nil
}

func (g gitlabRegistrar) Create(ctx context.Context, scope store.HookScope, spec Spec) (int64, error) {
	return g.client.CreateHook(ctx, gitlabScope(scope), gitlabSpec(spec))
}

func (g gitlabRegistrar) Update(ctx context.Context, scope store.HookScope, id int64, spec Spec) error {
	return gone(g.client.UpdateHook(ctx, gitlabScope(scope), id, gitlabSpec(spec)))
}

func (g gitlabRegistrar) Delete(ctx context.Context, scope store.HookScope, id int64) error {
	return gone(g.client.DeleteHook(ctx, gitlabScope(scope), id))
}
