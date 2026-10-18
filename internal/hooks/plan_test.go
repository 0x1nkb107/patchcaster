package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

var (
	instance   = store.HookScope{Kind: scopeInstance}
	payments   = store.HookScope{Kind: scopeGroup, Path: "payments"}
	cards      = store.HookScope{Kind: scopeGroup, Path: "payments/cards"}
	paymentAPI = store.HookScope{Kind: scopeRepository, Path: "payments/api"}
	oldAPI     = store.HookScope{Kind: scopeRepository, Path: "payments-old/api"}
	billing    = store.HookScope{Kind: scopeGroup, Path: "billing"}
)

func TestNeedRegistersOnlyScopesNoBroaderOneCovers(t *testing.T) {
	tests := []struct {
		name   string
		scopes []store.HookScope
		want   []Want
	}{
		{
			name:   "nothing",
			scopes: nil,
			want:   nil,
		},
		{
			name:   "a repository alone",
			scopes: []store.HookScope{paymentAPI},
			want:   []Want{{Scope: paymentAPI, Events: allEvents}},
		},
		{
			name:   "a group covers its repository and subgroup",
			scopes: []store.HookScope{paymentAPI, cards, payments},
			want:   []Want{{Scope: payments, Events: allEvents}},
		},
		{
			name:   "a group covers no namesake with a longer name",
			scopes: []store.HookScope{payments, oldAPI},
			want: []Want{
				{Scope: payments, Events: allEvents},
				{Scope: oldAPI, Events: allEvents},
			},
		},
		{
			name:   "siblings need one hook each, ordered by scope",
			scopes: []store.HookScope{payments, billing},
			want: []Want{
				{Scope: billing, Events: allEvents},
				{Scope: payments, Events: allEvents},
			},
		},
		{
			name:   "the instance takes merge requests and pushes, the narrow hooks the rest",
			scopes: []store.HookScope{paymentAPI, instance, payments},
			want: []Want{
				{Scope: instance, Events: instanceEvents},
				{Scope: payments, Events: narrowedEvents},
			},
		},
		{
			name:   "the instance alone",
			scopes: []store.HookScope{instance},
			want:   []Want{{Scope: instance, Events: instanceEvents}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			got := Need(tc.scopes)

			// assert
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestStateOfTellsHowASourcesEventsArrive(t *testing.T) {
	registered := store.ForgeHook{Scope: payments, ForgeHookID: 7}
	refused := store.ForgeHook{Scope: billing, LastError: "gitlab answered http 404: 404 Group Not Found"}
	tests := []struct {
		name      string
		scope     store.HookScope
		apiAccess bool
		want      SourceHook
	}{
		{"covered by a broader source", paymentAPI, true, SourceHook{State: Covered, CoveredBy: payments}},
		{"its own hook is registered", payments, true, SourceHook{State: Registered}},
		{"its hook was refused", billing, true, SourceHook{State: Failed, LastError: refused.LastError}},
		{"its hook is still to come", oldAPI, true, SourceHook{State: Pending}},
		{"without API access", oldAPI, false, SourceHook{State: Manual}},
		{"the instance is no broader scope here", cards, true, SourceHook{State: Covered, CoveredBy: payments}},
	}
	scopes := []store.HookScope{instance, payments, cards, paymentAPI, billing, oldAPI}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			got := StateOf(tc.scope, scopes, []store.ForgeHook{registered, refused}, tc.apiAccess)

			// assert
			assert.Equal(t, tc.want, got)
		})
	}
}
