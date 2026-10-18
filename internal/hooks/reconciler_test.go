package hooks

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/seal"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

const (
	testConnection = 3
	testURL        = "https://hooks.example.org/webhooks/3"
)

var reconcilerNow = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// --- fake store ------------------------------------------------------------

type fakeStore struct {
	connection *store.Connection
	sources    []store.SourceScope
	hooks      map[store.HookScope]store.ForgeHook
	tokens     map[int64][]byte // sealed tokens by id
	nextToken  int64
	busy       bool
	errRecord  error
}

func newFakeStore(credential []byte, scopes ...store.HookScope) *fakeStore {
	f := &fakeStore{
		connection: &store.Connection{ID: testConnection, Kind: "gitlab", Credential: credential},
		hooks:      map[store.HookScope]store.ForgeHook{},
		tokens:     map[int64][]byte{},
		nextToken:  100,
	}
	for i, scope := range scopes {
		f.sources = append(f.sources, store.SourceScope{SourceID: int64(i + 1), Scope: scope})
	}
	return f
}

func (f *fakeStore) Connection(_ context.Context, id int64) (*store.Connection, error) {
	if id != f.connection.ID {
		return nil, store.ErrNotFound
	}
	return f.connection, nil
}

func (f *fakeStore) Connections(context.Context, int64, int) ([]store.Connection, error) {
	return []store.Connection{*f.connection}, nil
}

func (f *fakeStore) SourceScopes(context.Context, int64) ([]store.SourceScope, error) {
	return f.sources, nil
}

func (f *fakeStore) ForgeHooks(context.Context, int64) ([]store.ForgeHook, error) {
	hooks := make([]store.ForgeHook, 0, len(f.hooks))
	for _, h := range f.hooks {
		hooks = append(hooks, h)
	}
	return hooks, nil
}

func (f *fakeStore) ReserveSigningTokenID(context.Context) (int64, error) {
	f.nextToken++
	return f.nextToken, nil
}

func (f *fakeStore) RecordHookRegistered(_ context.Context, _ store.Actor, h store.NewForgeHook, now time.Time) error {
	if f.errRecord != nil {
		return f.errRecord
	}
	f.tokens[h.Token.ID] = h.Token.Sealed
	f.hooks[h.Scope] = store.ForgeHook{
		ConnectionID:   h.ConnectionID,
		Scope:          h.Scope,
		Events:         h.Events,
		URL:            h.URL,
		ForgeHookID:    h.ForgeHookID,
		SigningTokenID: h.Token.ID,
		RegisteredAt:   now,
	}
	return nil
}

func (f *fakeStore) RecordHookChanged(_ context.Context, _ store.Actor, _ int64, scope store.HookScope, events store.HookEvents, url string, _ time.Time) error {
	h := f.hooks[scope]
	h.Events, h.URL, h.LastError = events, url, ""
	f.hooks[scope] = h
	return nil
}

func (f *fakeStore) RecordHookFailed(_ context.Context, connectionID int64, scope store.HookScope, events store.HookEvents, url, reason string, _ time.Time) error {
	h, known := f.hooks[scope]
	if !known {
		h = store.ForgeHook{ConnectionID: connectionID, Scope: scope, Events: events, URL: url}
	}
	h.LastError = reason
	f.hooks[scope] = h
	return nil
}

func (f *fakeStore) ForgetHook(_ context.Context, _ store.Actor, _ int64, scope store.HookScope, _ time.Time) error {
	delete(f.tokens, f.hooks[scope].SigningTokenID)
	delete(f.hooks, scope)
	return nil
}

func (f *fakeStore) LockHooks(context.Context, int64) (func(), bool, error) {
	if f.busy {
		return nil, false, nil
	}
	return func() {}, true, nil
}

// --- fake registrar --------------------------------------------------------

type forgeCall struct {
	op    string // create, update or delete
	scope store.HookScope
	id    int64
	spec  Spec
}

type fakeRegistrar struct {
	calls     []forgeCall
	listed    map[store.HookScope][]ForgeHook
	nextID    int64
	errCreate error
	errUpdate error
	errDelete error
}

func (f *fakeRegistrar) Hooks(_ context.Context, scope store.HookScope) ([]ForgeHook, error) {
	return f.listed[scope], nil
}

func (f *fakeRegistrar) Create(_ context.Context, scope store.HookScope, spec Spec) (int64, error) {
	f.calls = append(f.calls, forgeCall{op: "create", scope: scope, spec: spec})
	if f.errCreate != nil {
		return 0, f.errCreate
	}
	f.nextID++
	return f.nextID, nil
}

func (f *fakeRegistrar) Update(_ context.Context, scope store.HookScope, id int64, spec Spec) error {
	f.calls = append(f.calls, forgeCall{op: "update", scope: scope, id: id, spec: spec})
	return f.errUpdate
}

func (f *fakeRegistrar) Delete(_ context.Context, scope store.HookScope, id int64) error {
	f.calls = append(f.calls, forgeCall{op: "delete", scope: scope, id: id})
	return f.errDelete
}

type fakeRegistrars struct{ registrar *fakeRegistrar }

func (f fakeRegistrars) Registrar(context.Context, *store.Connection) (Registrar, error) {
	return f.registrar, nil
}

// --- harness -----------------------------------------------------------------

func newTestBox(t *testing.T) *seal.Box {
	t.Helper()
	key, err := seal.ParseKey(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	require.NoError(t, err)
	return seal.NewBox(key)
}

func newTestReconciler(t *testing.T, st *fakeStore, forge *fakeRegistrar) *Reconciler {
	t.Helper()
	return NewReconciler(st, fakeRegistrars{registrar: forge}, newTestBox(t), Config{
		Interval:   time.Hour,
		Busy:       time.Hour,
		WebhookURL: func(int64) string { return testURL },
	}, slog.New(slog.DiscardHandler), func() time.Time { return reconcilerNow })
}

// ops lists the forge calls as "op kind path".
func ops(calls []forgeCall) []string {
	var listed []string
	for _, c := range calls {
		listed = append(listed, c.op+" "+c.scope.Kind+" "+c.scope.Path)
	}
	return listed
}

// --- decisions -----------------------------------------------------------------

func TestAWantedHookIsRegisteredWithANewSigningToken(t *testing.T) {
	// arrange
	st := newFakeStore([]byte("sealed"), payments)
	forge := &fakeRegistrar{}
	r := newTestReconciler(t, st, forge)

	// act
	require.NoError(t, r.Reconcile(context.Background(), testConnection))

	// assert
	require.Equal(t, []string{"create group payments"}, ops(forge.calls))
	spec := forge.calls[0].spec
	assert.Equal(t, testURL, spec.URL)
	assert.Equal(t, hookName, spec.Name)
	assert.Equal(t, allEvents, spec.Events)
	require.True(t, strings.HasPrefix(spec.SigningToken, "whsec_"))
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(spec.SigningToken, "whsec_"))
	require.NoError(t, err)
	assert.Len(t, key, 32, "GitLab takes a 32-byte key")

	recorded := st.hooks[payments]
	assert.Equal(t, int64(1), recorded.ForgeHookID)
	opened, err := newTestBox(t).Open(st.tokens[recorded.SigningTokenID], store.SigningTokenBinding(recorded.SigningTokenID))
	require.NoError(t, err)
	assert.Equal(t, spec.SigningToken, string(opened), "the stored token verifies the hook's deliveries")
}

func TestABroaderHookReplacesNarrowerOnesAfterItIsRegistered(t *testing.T) {
	// arrange: the repository's hook exists, then its group becomes a source.
	st := newFakeStore([]byte("sealed"), paymentAPI, payments)
	st.hooks[paymentAPI] = store.ForgeHook{ConnectionID: testConnection, Scope: paymentAPI, Events: allEvents, URL: testURL, ForgeHookID: 41, SigningTokenID: 9}
	st.tokens[9] = []byte("sealed")
	forge := &fakeRegistrar{nextID: 50}
	r := newTestReconciler(t, st, forge)

	// act
	require.NoError(t, r.Reconcile(context.Background(), testConnection))

	// assert
	assert.Equal(t, []string{"create group payments", "delete repository payments/api"}, ops(forge.calls),
		"the broader hook comes first, so that no event falls into a gap")
	assert.Equal(t, int64(41), forge.calls[1].id)
	assert.NotContains(t, st.hooks, paymentAPI)
	assert.NotContains(t, st.tokens, int64(9), "the narrower hook's token goes with it")
}

func TestAnInstanceSourceNarrowsTheOtherHooks(t *testing.T) {
	// arrange
	st := newFakeStore([]byte("sealed"), payments, instance)
	st.hooks[payments] = store.ForgeHook{ConnectionID: testConnection, Scope: payments, Events: allEvents, URL: testURL, ForgeHookID: 41}
	forge := &fakeRegistrar{nextID: 50}
	r := newTestReconciler(t, st, forge)

	// act
	require.NoError(t, r.Reconcile(context.Background(), testConnection))

	// assert
	assert.Equal(t, []string{"create instance ", "update group payments"}, ops(forge.calls))
	assert.Equal(t, instanceEvents, forge.calls[0].spec.Events)
	assert.Equal(t, narrowedEvents, forge.calls[1].spec.Events)
	assert.Empty(t, forge.calls[1].spec.SigningToken, "an update keeps the hook's token")
	assert.Equal(t, narrowedEvents, st.hooks[payments].Events)
}

func TestAMovedWebhookAddressIsUpdated(t *testing.T) {
	// arrange
	st := newFakeStore([]byte("sealed"), payments)
	st.hooks[payments] = store.ForgeHook{ConnectionID: testConnection, Scope: payments, Events: allEvents, URL: "https://old.example/webhooks/3", ForgeHookID: 41}
	forge := &fakeRegistrar{}
	r := newTestReconciler(t, st, forge)

	// act
	require.NoError(t, r.Reconcile(context.Background(), testConnection))

	// assert
	assert.Equal(t, []string{"update group payments"}, ops(forge.calls))
	assert.Equal(t, testURL, st.hooks[payments].URL)
}

func TestAHookRemovedByHandIsRegisteredAnew(t *testing.T) {
	// arrange
	st := newFakeStore([]byte("sealed"), payments, instance)
	st.hooks[payments] = store.ForgeHook{ConnectionID: testConnection, Scope: payments, Events: allEvents, URL: testURL, ForgeHookID: 41}
	st.hooks[instance] = store.ForgeHook{ConnectionID: testConnection, Scope: instance, Events: instanceEvents, URL: testURL, ForgeHookID: 40}
	forge := &fakeRegistrar{nextID: 50, errUpdate: ErrHookGone}
	r := newTestReconciler(t, st, forge)

	// act
	require.NoError(t, r.Reconcile(context.Background(), testConnection))

	// assert
	assert.Equal(t, []string{"update group payments", "create group payments"}, ops(forge.calls))
	assert.Equal(t, int64(51), st.hooks[payments].ForgeHookID)
}

func TestARefusalIsRecordedWithTheForgesAnswer(t *testing.T) {
	// arrange
	st := newFakeStore([]byte("sealed"), payments)
	forge := &fakeRegistrar{errCreate: errors.New("gitlab answered http 404: 404 Group Not Found")}
	r := newTestReconciler(t, st, forge)

	// act
	require.NoError(t, r.Reconcile(context.Background(), testConnection))

	// assert
	h := st.hooks[payments]
	assert.Zero(t, h.ForgeHookID)
	assert.Equal(t, "gitlab answered http 404: 404 Group Not Found", h.LastError)
	assert.Empty(t, st.tokens, "a refused hook keeps no token")
}

func TestAnUnrecordedHookIsTakenBack(t *testing.T) {
	// arrange
	st := newFakeStore([]byte("sealed"), payments)
	st.errRecord = errors.New("database unavailable")
	forge := &fakeRegistrar{}
	r := newTestReconciler(t, st, forge)

	// act
	require.NoError(t, r.Reconcile(context.Background(), testConnection))

	// assert
	assert.Equal(t, []string{"create group payments", "delete group payments"}, ops(forge.calls),
		"without its token, nothing could verify the hook's deliveries")
}

func TestStraysOfAnInterruptedAttemptGoFirst(t *testing.T) {
	// arrange
	st := newFakeStore([]byte("sealed"), payments)
	forge := &fakeRegistrar{nextID: 50, listed: map[store.HookScope][]ForgeHook{payments: {
		{ID: 7, URL: testURL, Name: hookName},
		{ID: 8, URL: testURL, Name: "registered by hand"},
		{ID: 9, URL: "https://another-instance.example/webhooks/3", Name: hookName},
	}}}
	r := newTestReconciler(t, st, forge)

	// act
	require.NoError(t, r.Reconcile(context.Background(), testConnection))

	// assert
	assert.Equal(t, []string{"delete group payments", "create group payments"}, ops(forge.calls))
	assert.Equal(t, int64(7), forge.calls[0].id, "only a hook with PatchCaster's name and this address is a stray")
}

func TestAFailedRemovalKeepsTheRecord(t *testing.T) {
	// arrange
	st := newFakeStore([]byte("sealed"))
	st.hooks[payments] = store.ForgeHook{ConnectionID: testConnection, Scope: payments, Events: allEvents, URL: testURL, ForgeHookID: 41}
	forge := &fakeRegistrar{errDelete: errors.New("gitlab answered http 500")}
	r := newTestReconciler(t, st, forge)

	// act
	require.NoError(t, r.Reconcile(context.Background(), testConnection))

	// assert
	require.Contains(t, st.hooks, payments, "the hook stays known until it is gone at the forge")
	assert.Equal(t, "gitlab answered http 500", st.hooks[payments].LastError)
}

func TestAHookGoneAtTheForgeIsForgotten(t *testing.T) {
	// arrange
	st := newFakeStore([]byte("sealed"))
	st.hooks[payments] = store.ForgeHook{ConnectionID: testConnection, Scope: payments, ForgeHookID: 41}
	forge := &fakeRegistrar{errDelete: ErrHookGone}
	r := newTestReconciler(t, st, forge)

	// act
	require.NoError(t, r.Reconcile(context.Background(), testConnection))

	// assert
	assert.Empty(t, st.hooks)
}

func TestWithoutAPIAccessTheForgeStaysUntouched(t *testing.T) {
	// arrange
	st := newFakeStore(nil, billing)
	st.hooks[payments] = store.ForgeHook{ConnectionID: testConnection, Scope: payments, ForgeHookID: 41}
	st.hooks[cards] = store.ForgeHook{ConnectionID: testConnection, Scope: cards, LastError: "refused"}
	forge := &fakeRegistrar{}
	r := newTestReconciler(t, st, forge)

	// act
	require.NoError(t, r.Reconcile(context.Background(), testConnection))

	// assert
	assert.Empty(t, forge.calls)
	assert.Contains(t, st.hooks, payments, "a registered hook stays until it can be removed")
	assert.NotContains(t, st.hooks, cards, "a failure of a scope no source needs goes")
}

func TestABusyConnectionIsLeftToItsHolder(t *testing.T) {
	// arrange
	st := newFakeStore([]byte("sealed"), payments)
	st.busy = true
	forge := &fakeRegistrar{}
	r := newTestReconciler(t, st, forge)

	// act
	err := r.Reconcile(context.Background(), testConnection)

	// assert
	require.NoError(t, err)
	assert.Empty(t, forge.calls)
}
