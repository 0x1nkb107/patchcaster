package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

// hookLockClass keys the advisory locks that let one replica at a time
// register the hooks of a connection. The value spells "PCKH".
const hookLockClass int32 = 0x50434B48

// HookScope names where a hook takes events from, as a source names its
// scope: the instance, a group or a repository, by path. The instance has
// an empty path.
type HookScope struct {
	Kind string
	Path string
}

// HookEvents are the triggers of a hook.
type HookEvents struct {
	MergeRequests bool
	Pushes        bool
	Comments      bool
	Pipelines     bool
}

// ForgeHook is a hook PatchCaster registered for a forge connection, or is
// to register or to remove (SYSREQ-044). ForgeHookID is zero until the
// forge registered it. Until then, Events and URL are what the hook is to
// get. LastError says why the last attempt failed, and is empty after a
// success.
type ForgeHook struct {
	ConnectionID   int64
	Scope          HookScope
	Events         HookEvents
	URL            string
	ForgeHookID    int64
	SigningTokenID int64
	LastError      string
	AttemptedAt    time.Time
	RegisteredAt   time.Time
}

// SourceScope is the scope of one source.
type SourceScope struct {
	SourceID int64
	Scope    HookScope
}

// NewForgeHook is a hook the forge just registered, with the signing token
// PatchCaster generated for it, sealed for SigningTokenBinding.
type NewForgeHook struct {
	ConnectionID int64
	Scope        HookScope
	Events       HookEvents
	URL          string
	ForgeHookID  int64
	Token        NewSigningToken
}

func forgeHookFrom(row sqlcgen.ForgeHook) ForgeHook {
	return ForgeHook{
		ConnectionID: row.ConnectionID,
		Scope:        HookScope{Kind: row.ScopeKind, Path: row.ScopePath},
		Events: HookEvents{
			MergeRequests: row.MergeRequests,
			Pushes:        row.Pushes,
			Comments:      row.Comments,
			Pipelines:     row.Pipelines,
		},
		URL:            row.Url,
		ForgeHookID:    row.ForgeHookID.Int64,
		SigningTokenID: row.SigningTokenID.Int64,
		LastError:      row.LastError.String,
		AttemptedAt:    row.AttemptedAt,
		RegisteredAt:   row.RegisteredAt.Time,
	}
}

// hookTarget is how the audit log shows a hook.
func hookTarget(scope HookScope, forgeHookID, signingTokenID int64) map[string]any {
	return map[string]any{
		"scope_kind":    scope.Kind,
		"scope_path":    scope.Path,
		"forge_hook_id": forgeHookID,
		"signing_token": signingTokenID,
	}
}

// ForgeHooks reads the hooks of a connection, registered or not.
func (p *Postgres) ForgeHooks(ctx context.Context, connectionID int64) ([]ForgeHook, error) {
	rows, err := p.base.ForgeHookList(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("list hooks of connection %d: %w", connectionID, err)
	}
	hooks := make([]ForgeHook, 0, len(rows))
	for _, row := range rows {
		hooks = append(hooks, forgeHookFrom(row))
	}
	return hooks, nil
}

// SourceScopes lists the scopes of every source of a forge connection.
func (p *Postgres) SourceScopes(ctx context.Context, connectionID int64) ([]SourceScope, error) {
	rows, err := p.base.SourceScopesOfConnection(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("list source scopes of connection %d: %w", connectionID, err)
	}
	scopes := make([]SourceScope, 0, len(rows))
	for _, row := range rows {
		scopes = append(scopes, SourceScope{
			SourceID: row.ID,
			Scope:    HookScope{Kind: row.ScopeKind, Path: row.ScopePath},
		})
	}
	return scopes, nil
}

// RecordHookRegistered stores a hook the forge registered together with its
// signing token, and records both in the audit log, without the token's
// value.
func (p *Postgres) RecordHookRegistered(ctx context.Context, actor Actor, h NewForgeHook, now time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		err := q.SigningTokenAdd(ctx, sqlcgen.SigningTokenAddParams{
			ID:           h.Token.ID,
			ConnectionID: h.ConnectionID,
			Label:        h.Token.Label,
			Token:        h.Token.Sealed,
			CreatedAt:    now,
		})
		if err != nil {
			return referenceProblem(fmt.Errorf("store signing token of hook: %w", err), nil)
		}
		err = q.ForgeHookRegistered(ctx, sqlcgen.ForgeHookRegisteredParams{
			ConnectionID:   h.ConnectionID,
			ScopeKind:      h.Scope.Kind,
			ScopePath:      h.Scope.Path,
			MergeRequests:  h.Events.MergeRequests,
			Pushes:         h.Events.Pushes,
			Comments:       h.Events.Comments,
			Pipelines:      h.Events.Pipelines,
			Url:            h.URL,
			ForgeHookID:    sql.NullInt64{Int64: h.ForgeHookID, Valid: true},
			SigningTokenID: sql.NullInt64{Int64: h.Token.ID, Valid: true},
			At:             now,
		})
		if err != nil {
			return fmt.Errorf("store hook: %w", err)
		}
		return audit(ctx, q, actor, now, change{
			action:     "forge_hook.register",
			targetKind: "connection",
			targetID:   connectionTargetID(h.ConnectionID),
			after:      hookTarget(h.Scope, h.ForgeHookID, h.Token.ID),
		})
	})
}

// RecordHookChanged stores the new triggers and address of a registered
// hook, and records the change in the audit log. It returns ErrNotFound
// for a hook that is not registered.
func (p *Postgres) RecordHookChanged(ctx context.Context, actor Actor, connectionID int64, scope HookScope, events HookEvents, url string, now time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		n, err := q.ForgeHookChanged(ctx, sqlcgen.ForgeHookChangedParams{
			MergeRequests: events.MergeRequests,
			Pushes:        events.Pushes,
			Comments:      events.Comments,
			Pipelines:     events.Pipelines,
			Url:           url,
			At:            now,
			ConnectionID:  connectionID,
			ScopeKind:     scope.Kind,
			ScopePath:     scope.Path,
		})
		if err != nil {
			return fmt.Errorf("store changed hook: %w", err)
		}
		if n == 0 {
			return ErrNotFound
		}
		return audit(ctx, q, actor, now, change{
			action:     "forge_hook.change",
			targetKind: "connection",
			targetID:   connectionTargetID(connectionID),
			after: map[string]any{
				"scope_kind": scope.Kind,
				"scope_path": scope.Path,
				"url":        url,
			},
		})
	})
}

// RecordHookFailed stores why registering, changing or removing the hook
// of scope failed. A scope without a row gets one with the hook it is to
// get. The error must name no secret.
func (p *Postgres) RecordHookFailed(ctx context.Context, connectionID int64, scope HookScope, events HookEvents, url, reason string, now time.Time) error {
	err := p.base.ForgeHookFailed(ctx, sqlcgen.ForgeHookFailedParams{
		ConnectionID:  connectionID,
		ScopeKind:     scope.Kind,
		ScopePath:     scope.Path,
		MergeRequests: events.MergeRequests,
		Pushes:        events.Pushes,
		Comments:      events.Comments,
		Pipelines:     events.Pipelines,
		Url:           url,
		Error:         sql.NullString{String: truncated(reason, maxHealthError), Valid: true},
		At:            now,
	})
	if err != nil {
		return fmt.Errorf("store hook failure: %w", err)
	}
	return nil
}

// ForgetHook removes the row of the hook of scope with its signing token,
// once the hook is gone at the forge or was never registered, and records
// a removed hook in the audit log.
func (p *Postgres) ForgetHook(ctx context.Context, actor Actor, connectionID int64, scope HookScope, now time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		tokenID, err := q.ForgeHookDelete(ctx, sqlcgen.ForgeHookDeleteParams{
			ConnectionID: connectionID,
			ScopeKind:    scope.Kind,
			ScopePath:    scope.Path,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("remove hook: %w", err)
		}
		if !tokenID.Valid {
			return nil
		}
		_, err = q.SigningTokenRemove(ctx, sqlcgen.SigningTokenRemoveParams{ID: tokenID.Int64, ConnectionID: connectionID})
		if err != nil {
			return fmt.Errorf("remove signing token of hook: %w", err)
		}
		return audit(ctx, q, actor, now, change{
			action:     "forge_hook.remove",
			targetKind: "connection",
			targetID:   connectionTargetID(connectionID),
			before: map[string]any{
				"scope_kind":    scope.Kind,
				"scope_path":    scope.Path,
				"signing_token": tokenID.Int64,
			},
		})
	})
}

// LockHooks takes the lock on registering the hooks of a connection,
// unless another replica holds it. The lock lasts until release, or until
// the database session ends, so a replica that dies frees it.
func (p *Postgres) LockHooks(ctx context.Context, connectionID int64) (release func(), locked bool, err error) {
	if connectionID > math.MaxInt32 {
		return nil, false, fmt.Errorf("lock hooks of connection %d: the id exceeds the lock's range", connectionID)
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin hook lock: %w", err)
	}
	locked, err = p.base.WithTx(tx).ForgeHookLock(ctx, sqlcgen.ForgeHookLockParams{
		Class:        hookLockClass,
		ConnectionID: int32(connectionID),
	})
	if err != nil || !locked {
		_ = tx.Rollback()
		if err != nil {
			return nil, false, fmt.Errorf("lock hooks of connection %d: %w", connectionID, err)
		}
		return nil, false, nil
	}
	return func() { _ = tx.Rollback() }, true, nil
}
