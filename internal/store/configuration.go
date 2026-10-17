package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

var (
	// ErrNotFound reports that the item does not exist.
	ErrNotFound = errors.New("not found")
	// ErrVersionMismatch reports that the item changed since the caller read
	// it, so the change was refused rather than lost (ADR-043).
	ErrVersionMismatch = errors.New("the item changed since it was read")
)

// Actor names who makes a change, for the audit log. PersonID is zero for
// the commands that act with the database credentials alone.
type Actor struct {
	PersonID int64
	Label    string
}

// AuditEntry is one recorded change (SYSREQ-041). Before and After hold JSON,
// with secrets already redacted, or null.
type AuditEntry struct {
	ID            int64
	At            time.Time
	ActorPersonID int64
	ActorLabel    string
	Action        string
	TargetKind    string
	TargetID      string
	Before        json.RawMessage
	After         json.RawMessage
}

// change is one audit entry to write along with the change it describes.
// Before and after are encoded as JSON, so they must never hold a secret.
type change struct {
	action     string
	targetKind string
	targetID   string
	before     any
	after      any
}

// inTx runs fn in one transaction, which commits only when fn succeeds. Every
// configuration change runs in one, so that it commits together with its
// audit entry or not at all (SYSREQ-041).
func (p *Postgres) inTx(ctx context.Context, fn func(q *sqlcgen.Queries) error) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(sqlcgen.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// audit appends the entry for a change to the transaction q belongs to.
func audit(ctx context.Context, q *sqlcgen.Queries, actor Actor, now time.Time, c change) error {
	before, err := json.Marshal(c.before)
	if err != nil {
		return fmt.Errorf("encode audit state before %s: %w", c.action, err)
	}
	after, err := json.Marshal(c.after)
	if err != nil {
		return fmt.Errorf("encode audit state after %s: %w", c.action, err)
	}
	err = q.AuditAppend(ctx, sqlcgen.AuditAppendParams{
		At: now,
		ActorPersonID: sql.NullInt64{
			Int64: actor.PersonID,
			Valid: actor.PersonID != 0,
		},
		ActorLabel: actor.Label,
		Action:     c.action,
		TargetKind: c.targetKind,
		TargetID:   c.targetID,
		Before:     before,
		After:      after,
	})
	if err != nil {
		return fmt.Errorf("record %s in the audit log: %w", c.action, err)
	}
	return nil
}

// AuditEntries pages through the audit log, newest first, starting before
// the entry with beforeID, or at the newest when beforeID is zero.
func (p *Postgres) AuditEntries(ctx context.Context, beforeID int64, limit int) ([]AuditEntry, error) {
	rows, err := p.base.AuditList(ctx, sqlcgen.AuditListParams{
		BeforeID: beforeID,
		PageSize: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list audit log: %w", err)
	}
	entries := make([]AuditEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, AuditEntry{
			ID:            row.ID,
			At:            row.At,
			ActorPersonID: row.ActorPersonID.Int64,
			ActorLabel:    row.ActorLabel,
			Action:        row.Action,
			TargetKind:    row.TargetKind,
			TargetID:      row.TargetID,
			Before:        row.Before,
			After:         row.After,
		})
	}
	return entries, nil
}
