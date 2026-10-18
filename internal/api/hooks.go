package api

import (
	"context"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/hooks"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// Hooks takes note that the sources or the credential of a forge
// connection changed, so that its hooks follow (SYSREQ-044). The
// reconciler of package hooks is the real one, and it works in the
// background.
type Hooks interface {
	Poke(connectionID int64)
}

func scopeOf(scope store.HookScope) v1gen.SourceScope {
	rendered := v1gen.SourceScope{Kind: v1gen.SourceScopeKind(scope.Kind)}
	if scope.Path != "" {
		rendered.Path = v1gen.NewOptString(scope.Path)
	}
	return rendered
}

func sourceHookOf(h hooks.SourceHook) v1gen.SourceHook {
	rendered := v1gen.SourceHook{
		State:        v1gen.SourceHookState(h.State),
		LastError:    optText(h.LastError),
		RegisteredAt: optTime(h.RegisteredAt),
	}
	if h.State == hooks.Covered {
		rendered.CoveredBy = v1gen.NewOptSourceScope(scopeOf(h.CoveredBy))
	}
	return rendered
}

// sourceHooks reads how the events of each source reach PatchCaster, by
// source id. A source's state depends on every source of its connection,
// so each connection is read once.
func (s *Service) sourceHooks(ctx context.Context, sources []store.Source) (map[int64]v1gen.SourceHook, error) {
	byConnection := make(map[int64][]store.Source)
	for _, src := range sources {
		byConnection[src.ConnectionID] = append(byConnection[src.ConnectionID], src)
	}
	states := make(map[int64]v1gen.SourceHook, len(sources))
	for connectionID, owned := range byConnection {
		connection, err := s.store.Connection(ctx, connectionID)
		if err != nil {
			return nil, err
		}
		siblings, err := s.store.SourceScopes(ctx, connectionID)
		if err != nil {
			return nil, err
		}
		recorded, err := s.store.ForgeHooks(ctx, connectionID)
		if err != nil {
			return nil, err
		}
		scopes := make([]store.HookScope, 0, len(siblings))
		for _, sibling := range siblings {
			scopes = append(scopes, sibling.Scope)
		}
		apiAccess := connection.Credential != nil
		for _, src := range owned {
			scope := store.HookScope{Kind: src.ScopeKind, Path: src.ScopePath}
			states[src.ID] = sourceHookOf(hooks.StateOf(scope, scopes, recorded, apiAccess))
		}
	}
	return states, nil
}
