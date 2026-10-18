package hooks

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/platform"
	"github.com/patchcaster-app/patchcaster/internal/seal"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// hookName names the hooks PatchCaster registers. Together with the
// connection's webhook address it identifies them at the forge, so it must
// not change.
const hookName = "PatchCaster"

// actor names the reconciler in the audit log.
var actor = store.Actor{Label: "hook registration"}

// ErrHookGone reports a hook the forge does not hold, such as one removed
// by hand.
var ErrHookGone = errors.New("the forge holds no such hook")

// Spec is a hook as PatchCaster registers it. SigningToken goes with the
// creation only.
type Spec struct {
	URL          string
	Name         string
	Description  string
	Events       store.HookEvents
	SigningToken string
}

// ForgeHook is a hook as the forge lists it.
type ForgeHook struct {
	ID   int64
	URL  string
	Name string
}

// Registrar registers hooks at the forge of one connection. Update and
// Delete return ErrHookGone for a hook the forge does not hold.
type Registrar interface {
	Hooks(ctx context.Context, scope store.HookScope) ([]ForgeHook, error)
	Create(ctx context.Context, scope store.HookScope, spec Spec) (int64, error)
	Update(ctx context.Context, scope store.HookScope, id int64, spec Spec) error
	Delete(ctx context.Context, scope store.HookScope, id int64) error
}

// Registrars opens the registrar of a forge connection with its stored
// credential.
type Registrars interface {
	Registrar(ctx context.Context, connection *store.Connection) (Registrar, error)
}

// Store is what the reconciler reads and records.
type Store interface {
	Connection(ctx context.Context, id int64) (*store.Connection, error)
	Connections(ctx context.Context, afterID int64, limit int) ([]store.Connection, error)
	SourceScopes(ctx context.Context, connectionID int64) ([]store.SourceScope, error)
	ForgeHooks(ctx context.Context, connectionID int64) ([]store.ForgeHook, error)
	ReserveSigningTokenID(ctx context.Context) (int64, error)
	RecordHookRegistered(ctx context.Context, actor store.Actor, h store.NewForgeHook, now time.Time) error
	RecordHookChanged(ctx context.Context, actor store.Actor, connectionID int64, scope store.HookScope, events store.HookEvents, url string, now time.Time) error
	RecordHookFailed(ctx context.Context, connectionID int64, scope store.HookScope, events store.HookEvents, url, reason string, now time.Time) error
	ForgetHook(ctx context.Context, actor store.Actor, connectionID int64, scope store.HookScope, now time.Time) error
	LockHooks(ctx context.Context, connectionID int64) (release func(), locked bool, err error)
}

// Config tunes a Reconciler.
type Config struct {
	// Interval is the time between two passes over every connection. A
	// pass retries what failed and notices a changed webhook address.
	Interval time.Duration
	// Busy is how long a poked connection waits when another replica
	// holds it.
	Busy time.Duration
	// WebhookURL returns the address a forge sends a connection's
	// webhooks to.
	WebhookURL func(connectionID int64) string
}

// Reconciler keeps the hooks at each forge in line with the sources of its
// connection (SYSREQ-044). A change of the sources pokes it, and it passes
// over every connection at start and at every interval. One replica at a
// time works on a connection.
type Reconciler struct {
	store      Store
	registrars Registrars
	box        *seal.Box
	cfg        Config
	log        *slog.Logger
	now        func() time.Time
	pokes      chan int64
}

// NewReconciler wires a reconciler. A nil clock defaults to time.Now.
func NewReconciler(st Store, registrars Registrars, box *seal.Box, cfg Config, log *slog.Logger, now func() time.Time) *Reconciler {
	if now == nil {
		now = time.Now
	}
	return &Reconciler{
		store:      st,
		registrars: registrars,
		box:        box,
		cfg:        cfg,
		log:        log,
		now:        now,
		pokes:      make(chan int64, 64),
	}
}

// Poke asks for the hooks of a connection to be reconciled soon. It never
// blocks: when the queue is full, the next pass takes the connection.
func (r *Reconciler) Poke(connectionID int64) {
	select {
	case r.pokes <- connectionID:
	default:
	}
}

// Run reconciles every connection at start and at every interval, and a
// poked connection at once, until ctx ends.
func (r *Reconciler) Run(ctx context.Context) error {
	r.pass(ctx)
	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case id := <-r.pokes:
			r.reconcileLogged(ctx, id)
		case <-ticker.C:
			r.pass(ctx)
		}
	}
}

// pass reconciles every forge connection.
func (r *Reconciler) pass(ctx context.Context) {
	var afterID int64
	for {
		connections, err := r.store.Connections(ctx, afterID, 100)
		if err != nil {
			r.log.Error("list connections for hook registration failed", "err", err)
			return
		}
		for _, c := range connections {
			if platform.Kind(c.Kind).IsForge() {
				r.reconcileLogged(ctx, c.ID)
			}
		}
		if len(connections) < 100 {
			return
		}
		afterID = connections[len(connections)-1].ID
	}
}

func (r *Reconciler) reconcileLogged(ctx context.Context, connectionID int64) {
	if err := r.Reconcile(ctx, connectionID); err != nil && ctx.Err() == nil {
		r.log.Error("hook registration failed", "connection", connectionID, "err", err)
	}
}

// Reconcile brings the hooks of one connection in line with its sources:
// it registers the hooks they need, changes those whose triggers or
// address moved, and removes the rest. A hook that fails is recorded with
// the forge's answer and retried at the next pass. When another replica
// holds the connection, the reconciler tries again a little later.
func (r *Reconciler) Reconcile(ctx context.Context, connectionID int64) error {
	release, locked, err := r.store.LockHooks(ctx, connectionID)
	if err != nil {
		return err
	}
	if !locked {
		time.AfterFunc(r.cfg.Busy, func() { r.Poke(connectionID) })
		return nil
	}
	defer release()

	connection, err := r.store.Connection(ctx, connectionID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !platform.Kind(connection.Kind).IsForge() {
		return nil
	}
	sources, err := r.store.SourceScopes(ctx, connectionID)
	if err != nil {
		return err
	}
	recorded, err := r.store.ForgeHooks(ctx, connectionID)
	if err != nil {
		return err
	}
	scopes := make([]store.HookScope, 0, len(sources))
	for _, s := range sources {
		scopes = append(scopes, s.Scope)
	}
	return r.converge(ctx, connection, Need(scopes), recorded)
}

// converge changes the forge from the recorded hooks to the wanted ones.
// New and changed hooks come first and removals last, so that no event
// falls into a gap while a broader hook replaces narrower ones.
func (r *Reconciler) converge(ctx context.Context, connection *store.Connection, wants []Want, recorded []store.ForgeHook) error {
	byScope := make(map[store.HookScope]store.ForgeHook, len(recorded))
	for _, h := range recorded {
		byScope[h.Scope] = h
	}
	wanted := make(map[store.HookScope]bool, len(wants))
	for _, w := range wants {
		wanted[w.Scope] = true
	}
	url := r.cfg.WebhookURL(connection.ID)

	// Without API access, nothing at the forge can change. Records of
	// hooks that were never registered and are no longer needed still go.
	if connection.Credential == nil {
		for _, h := range recorded {
			if !wanted[h.Scope] && h.ForgeHookID == 0 {
				r.forget(ctx, connection.ID, h.Scope)
			}
		}
		return nil
	}
	registrar, err := r.registrars.Registrar(ctx, connection)
	if err != nil {
		return fmt.Errorf("open registrar of connection %d: %w", connection.ID, err)
	}
	for _, w := range wants {
		h, known := byScope[w.Scope]
		switch {
		case !known || h.ForgeHookID == 0:
			r.create(ctx, registrar, connection.ID, w, url)
		case h.Events != w.Events || h.URL != url:
			r.change(ctx, registrar, h, w, url)
		}
	}
	for _, h := range recorded {
		if !wanted[h.Scope] {
			r.remove(ctx, registrar, h)
		}
	}
	return nil
}

// specOf is the hook that w calls for at url.
func specOf(connectionID int64, w Want, url string) Spec {
	return Spec{
		URL:         url,
		Name:        hookName,
		Description: "Registered by PatchCaster for connection " + strconv.FormatInt(connectionID, 10) + ". PatchCaster removes it when no source needs it.",
		Events:      w.Events,
	}
}

// newSigningToken generates a signing token in the form GitLab takes:
// "whsec_" and the base64 of a 32-byte key.
func newSigningToken() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate signing token: %w", err)
	}
	return "whsec_" + base64.StdEncoding.EncodeToString(key), nil
}

// tokenLabel names a hook's signing token in the connection's list.
func tokenLabel(scope store.HookScope) string {
	if scope.Kind == scopeInstance {
		return "hook registered by PatchCaster for the instance"
	}
	return "hook registered by PatchCaster for " + scope.Kind + " " + scope.Path
}

// create registers the hook w calls for, with a new signing token. Hooks
// of PatchCaster's own that an interrupted attempt left at the scope go
// first, so that no event arrives twice.
func (r *Reconciler) create(ctx context.Context, registrar Registrar, connectionID int64, w Want, url string) {
	if err := r.removeStrays(ctx, registrar, w.Scope, url); err != nil {
		r.failed(ctx, connectionID, w.Scope, w.Events, url, err)
		return
	}
	token, err := newSigningToken()
	if err != nil {
		r.failed(ctx, connectionID, w.Scope, w.Events, url, err)
		return
	}
	tokenID, err := r.store.ReserveSigningTokenID(ctx)
	if err != nil {
		r.failed(ctx, connectionID, w.Scope, w.Events, url, err)
		return
	}
	sealed, err := r.box.Seal([]byte(token), store.SigningTokenBinding(tokenID))
	if err != nil {
		r.failed(ctx, connectionID, w.Scope, w.Events, url, err)
		return
	}
	spec := specOf(connectionID, w, url)
	spec.SigningToken = token
	hookID, err := registrar.Create(ctx, w.Scope, spec)
	if err != nil {
		r.failed(ctx, connectionID, w.Scope, w.Events, url, err)
		return
	}
	err = r.store.RecordHookRegistered(ctx, actor, store.NewForgeHook{
		ConnectionID: connectionID,
		Scope:        w.Scope,
		Events:       w.Events,
		URL:          url,
		ForgeHookID:  hookID,
		Token: store.NewSigningToken{
			ID:           tokenID,
			ConnectionID: connectionID,
			Label:        tokenLabel(w.Scope),
			Sealed:       sealed,
		},
	}, r.now())
	if err != nil {
		// Without its token, nothing could verify the hook's deliveries.
		// Take it back, and try again at the next pass.
		if deleteErr := registrar.Delete(ctx, w.Scope, hookID); deleteErr != nil && !errors.Is(deleteErr, ErrHookGone) {
			r.log.Warn("removing an unrecorded hook failed; the next attempt removes it", "connection", connectionID, "scope", w.Scope, "err", deleteErr)
		}
		r.failed(ctx, connectionID, w.Scope, w.Events, url, err)
		return
	}
	r.log.Info("hook registered", "connection", connectionID, "scope", w.Scope, "hook", hookID)
}

// removeStrays removes PatchCaster's own hooks at scope that no record
// knows, which an interrupted registration may have left. They carry
// PatchCaster's name and the connection's address. A hook registered by
// hand never carries both.
func (r *Reconciler) removeStrays(ctx context.Context, registrar Registrar, scope store.HookScope, url string) error {
	hooks, err := registrar.Hooks(ctx, scope)
	if err != nil {
		return err
	}
	for _, h := range hooks {
		if h.Name != hookName || h.URL != url {
			continue
		}
		if err := registrar.Delete(ctx, scope, h.ID); err != nil && !errors.Is(err, ErrHookGone) {
			return err
		}
	}
	return nil
}

// change gives a registered hook the triggers and address w calls for. A
// hook removed by hand at the forge is registered anew.
func (r *Reconciler) change(ctx context.Context, registrar Registrar, h store.ForgeHook, w Want, url string) {
	err := registrar.Update(ctx, h.Scope, h.ForgeHookID, specOf(h.ConnectionID, w, url))
	if errors.Is(err, ErrHookGone) {
		if r.forget(ctx, h.ConnectionID, h.Scope) {
			r.create(ctx, registrar, h.ConnectionID, w, url)
		}
		return
	}
	if err != nil {
		r.failed(ctx, h.ConnectionID, h.Scope, w.Events, url, err)
		return
	}
	if err := r.store.RecordHookChanged(ctx, actor, h.ConnectionID, h.Scope, w.Events, url, r.now()); err != nil {
		r.log.Error("recording a changed hook failed", "connection", h.ConnectionID, "scope", h.Scope, "err", err)
		return
	}
	r.log.Info("hook changed", "connection", h.ConnectionID, "scope", h.Scope, "hook", h.ForgeHookID)
}

// remove takes a hook no source needs off the forge, then forgets it with
// its signing token.
func (r *Reconciler) remove(ctx context.Context, registrar Registrar, h store.ForgeHook) {
	if h.ForgeHookID != 0 {
		err := registrar.Delete(ctx, h.Scope, h.ForgeHookID)
		if err != nil && !errors.Is(err, ErrHookGone) {
			r.failed(ctx, h.ConnectionID, h.Scope, h.Events, h.URL, err)
			return
		}
	}
	if r.forget(ctx, h.ConnectionID, h.Scope) && h.ForgeHookID != 0 {
		r.log.Info("hook removed", "connection", h.ConnectionID, "scope", h.Scope, "hook", h.ForgeHookID)
	}
}

// forget removes a hook's record and reports whether that worked.
func (r *Reconciler) forget(ctx context.Context, connectionID int64, scope store.HookScope) bool {
	if err := r.store.ForgetHook(ctx, actor, connectionID, scope, r.now()); err != nil {
		r.log.Error("forgetting a hook failed", "connection", connectionID, "scope", scope, "err", err)
		return false
	}
	return true
}

// failed records why an attempt at scope failed, for the source's hook
// state and the next pass.
func (r *Reconciler) failed(ctx context.Context, connectionID int64, scope store.HookScope, events store.HookEvents, url string, cause error) {
	r.log.Warn("hook registration attempt failed", "connection", connectionID, "scope", scope, "err", cause)
	if err := r.store.RecordHookFailed(ctx, connectionID, scope, events, url, cause.Error(), r.now()); err != nil {
		r.log.Error("recording a hook failure failed", "connection", connectionID, "scope", scope, "err", err)
	}
}
