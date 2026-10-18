// Package hooks registers the hooks that deliver the events of a forge
// connection's sources (SYSREQ-044). Need derives the hooks the sources
// call for, and a Reconciler changes the forge until it holds exactly
// those (ADR-045).
package hooks

import (
	"slices"
	"strings"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

// The kinds of scope a source has.
const (
	scopeInstance   = "instance"
	scopeGroup      = "group"
	scopeRepository = "repository"
)

// Every trigger PatchCaster uses, and the ones an instance-wide hook can
// deliver. A GitLab system hook knows merge requests and pushes only.
var (
	allEvents      = store.HookEvents{MergeRequests: true, Pushes: true, Comments: true, Pipelines: true}
	instanceEvents = store.HookEvents{MergeRequests: true, Pushes: true}
	narrowedEvents = store.HookEvents{Comments: true, Pipelines: true}
)

// Want is a hook that the sources of a connection need.
type Want struct {
	Scope  store.HookScope
	Events store.HookEvents
}

// covers reports whether a hook of outer takes the events of inner too: a
// group takes those of its subgroups and repositories, and every scope
// takes its own. The instance is left out, because its hook takes only
// some of the events.
func covers(outer, inner store.HookScope) bool {
	if outer == inner {
		return true
	}
	return outer.Kind == scopeGroup && strings.HasPrefix(inner.Path, outer.Path+"/")
}

// broaderScope returns the scope among scopes, other than scope itself,
// whose hook takes the events of scope. The broadest wins, so that the
// answer names the hook that is registered.
func broaderScope(scope store.HookScope, scopes []store.HookScope) (store.HookScope, bool) {
	var broadest store.HookScope
	found := false
	for _, outer := range scopes {
		if outer == scope || outer.Kind == scopeInstance || !covers(outer, scope) {
			continue
		}
		if !found || len(outer.Path) < len(broadest.Path) {
			broadest, found = outer, true
		}
	}
	return broadest, found
}

// Need derives the hooks that sources with scopes call for: one for every
// scope that no broader scope covers. An instance-wide scope takes merge
// requests and pushes everywhere, and the narrower scopes then keep hooks
// for comments and pipelines alone, so that no event arrives twice
// (owner decision 2026-10-18). The result is ordered by scope.
func Need(scopes []store.HookScope) []Want {
	hasInstance := slices.ContainsFunc(scopes, func(s store.HookScope) bool { return s.Kind == scopeInstance })
	narrowEvents := allEvents
	if hasInstance {
		narrowEvents = narrowedEvents
	}

	var wants []Want
	if hasInstance {
		wants = append(wants, Want{Scope: store.HookScope{Kind: scopeInstance}, Events: instanceEvents})
	}
	var narrow []store.HookScope
	for _, scope := range scopes {
		if scope.Kind == scopeInstance || slices.Contains(narrow, scope) {
			continue
		}
		if _, covered := broaderScope(scope, scopes); covered {
			continue
		}
		narrow = append(narrow, scope)
	}
	slices.SortFunc(narrow, func(a, b store.HookScope) int {
		return strings.Compare(a.Kind+"\x00"+a.Path, b.Kind+"\x00"+b.Path)
	})
	for _, scope := range narrow {
		wants = append(wants, Want{Scope: scope, Events: narrowEvents})
	}
	return wants
}

// State is how the events of a source reach PatchCaster.
type State string

// The states of a source's hook.
const (
	// Registered: the hook of the source's scope is registered.
	Registered State = "registered"
	// Pending: the hook is still to be registered.
	Pending State = "pending"
	// Failed: the last attempt to register the hook failed.
	Failed State = "failed"
	// Covered: the hook of a broader scope takes the source's events.
	Covered State = "covered"
	// Manual: without API access, the hook is registered by hand.
	Manual State = "manual"
)

// SourceHook tells how the events of one source reach PatchCaster.
// CoveredBy names the broader scope of a covered source. LastError says why
// the last attempt failed, also for a registered hook whose change failed.
type SourceHook struct {
	State        State
	CoveredBy    store.HookScope
	LastError    string
	RegisteredAt time.Time
}

// StateOf tells how the events of a source with scope reach PatchCaster,
// given the scopes of every source of its connection, the hooks recorded
// for the connection, and whether the connection has API access.
func StateOf(scope store.HookScope, scopes []store.HookScope, recorded []store.ForgeHook, apiAccess bool) SourceHook {
	if scope.Kind != scopeInstance {
		if broader, covered := broaderScope(scope, scopes); covered {
			return SourceHook{State: Covered, CoveredBy: broader}
		}
	}
	index := slices.IndexFunc(recorded, func(h store.ForgeHook) bool { return h.Scope == scope })
	if index < 0 {
		if apiAccess {
			return SourceHook{State: Pending}
		}
		return SourceHook{State: Manual}
	}
	hook := recorded[index]
	switch {
	case hook.ForgeHookID != 0:
		return SourceHook{State: Registered, LastError: hook.LastError, RegisteredAt: hook.RegisteredAt}
	case hook.LastError != "":
		return SourceHook{State: Failed, LastError: hook.LastError}
	default:
		return SourceHook{State: Pending}
	}
}
