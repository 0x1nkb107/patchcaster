// Package store persists the delivery queue, thread mappings, and seen-webhook
// records in PostgreSQL (SYSARCH-002). The API below is backend-neutral: no
// engine type crosses this boundary (ADR-010), so consumers depend only on
// these domain types.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/plan"
	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

// Job is one queued outbound message as claimed by the delivery worker.
type Job struct {
	ID          int64
	MRRef       string
	Action      string
	DestChannel string // frozen at ingress, immutable (SYSREQ-024)
	Payload     []byte // normalized event.Event JSON
	Attempts    int    // completed failed attempts

	// Delivery phases already completed (SYSREQ-028, ADR-014): a retry skips
	// them instead of repeating a posted notice.
	NoticePosted  bool
	RootRefreshed bool
}

// AcceptResult reports the outcome of an accept transaction.
type AcceptResult struct {
	// Accepted is false when the webhook identifier was already seen: a
	// concurrent duplicate won, and neither state nor queue was touched.
	Accepted bool
	// Enqueued reports whether a delivery row was written; false means the
	// delivery applied state (or attributed a push) without posting.
	Enqueued bool
	// Attributed counts merge requests given the commit count of a push
	// delivery (SYSREQ-030); zero for a push that matched no open request.
	Attributed int
}

// Event decodes the normalized event carried by the job.
func (j *Job) Event() (*event.Event, error) {
	var ev event.Event
	if err := json.Unmarshal(j.Payload, &ev); err != nil {
		return nil, fmt.Errorf("decode job %d payload: %w", j.ID, err)
	}
	return &ev, nil
}

// Thread is the Mattermost thread mapping for one merge request (SYSREQ-006).
type Thread struct {
	ChannelID  string
	RootPostID string
	Open       bool
}

// SettlementKind enumerates how the worker settled a claimed job.
type SettlementKind int

const (
	// SettlementSent: Mattermost accepted the post.
	SettlementSent SettlementKind = iota
	// SettlementSkipped: no post will ever be made for this job (SYSREQ-007).
	SettlementSkipped
	// SettlementRetried: deferred for another attempt (SYSREQ-016).
	SettlementRetried
	// SettlementParked: no further automatic attempts (SYSREQ-016/026).
	SettlementParked
)

// Settlement is the terminal-or-deferred outcome for one claim. Construct it
// with Sent, Skipped, Retried, or Parked.
type Settlement struct {
	Kind     SettlementKind
	Reason   string
	Attempts int
	Next     time.Time
	Err      string
}

// Sent settles the job as delivered at now.
func Sent() Settlement { return Settlement{Kind: SettlementSent} }

// Skipped settles the job as never-postable with the skip cause.
func Skipped(reason string) Settlement { return Settlement{Kind: SettlementSkipped, Reason: reason} }

// Retried defers the job to Next after recording Attempts failed attempts.
func Retried(attempts int, next time.Time, errText string) Settlement {
	return Settlement{Kind: SettlementRetried, Attempts: attempts, Next: next, Err: errText}
}

// Parked removes the job from the retry rotation with the park cause.
func Parked(reason string, attempts int, errText string) Settlement {
	return Settlement{Kind: SettlementParked, Reason: reason, Attempts: attempts, Err: errText}
}

// Postgres implements all persistence behind a database/sql connection pool
// driven by the pgx stdlib driver (pure Go, CGO stays off).
type Postgres struct {
	db   *sql.DB
	base *sqlcgen.Queries
}

// New binds the repository to an open database handle.
func New(db *sql.DB) *Postgres {
	return &Postgres{db: db, base: sqlcgen.New(db)}
}

// Ping checks store availability for the readiness probe.
func (p *Postgres) Ping(ctx context.Context) error {
	_, err := p.base.Ping(ctx)
	return err
}

// Seen reports whether a webhook identifier is already recorded.
func (p *Postgres) Seen(ctx context.Context, webhookID string) (bool, error) {
	return p.base.SeenExists(ctx, webhookID)
}

// MarkOutcome records a seen webhook that produced no queue row (discarded
// deliveries keep their decision, SYSREQ-014).
func (p *Postgres) MarkOutcome(ctx context.Context, webhookID, outcome string, now time.Time) error {
	_, err := p.base.MarkSeen(ctx, sqlcgen.MarkSeenParams{
		WebhookID: webhookID,
		Outcome:   outcome,
		FirstSeen: now,
	})
	if err != nil {
		return fmt.Errorf("mark webhook %q seen: %w", webhookID, err)
	}
	return nil
}

// Accept durably records the delivery as seen, applies its derived state, and
// enqueues its message — all in one transaction (SYSREQ-014, SYSREQ-017,
// SYSREQ-027). Ingest is the only writer of board state, so applying inside
// the seen-mark makes every counter exactly-once: the identifier admits one
// transaction, period. It reports (accepted, enqueued): accepted false means
// the identifier was already seen; enqueued false means the delivery moved no
// observable state, so no delivery row was needed. ev.Plan carries the
// ingest-side intent; Accept finalizes its Refresh flag from what actually
// changed.
func (p *Postgres) Accept(ctx context.Context, ev *event.Event, destChannel string, now time.Time) (AcceptResult, error) {
	if ev.Plan == nil {
		ev.Plan = &event.Plan{}
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return AcceptResult{}, fmt.Errorf("begin accept: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := sqlcgen.New(tx)

	res, err := q.MarkSeen(ctx, sqlcgen.MarkSeenParams{
		WebhookID: ev.WebhookID,
		Outcome:   "forwarded",
		FirstSeen: now,
	})
	if err != nil {
		return AcceptResult{}, fmt.Errorf("mark webhook %q seen: %w", ev.WebhookID, err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return AcceptResult{}, fmt.Errorf("inspect seen insert for %q: %w", ev.WebhookID, err)
	}
	if inserted == 0 {
		return AcceptResult{}, nil // duplicate: the earlier delivery owns state and queue row
	}

	// Push deliveries only attribute a commit count; they never own a merge
	// request row nor a delivery (SYSREQ-030).
	if ev.Kind == event.KindPush {
		attributed, err := q.StateAttributePush(ctx, sqlcgen.StateAttributePushParams{
			SourceProjectID: ev.PushProjectID,
			SourceBranch:    ev.PushBranch,
			LastPushCount:   int32(ev.PushCommits),
			UpdatedAt:       now,
		})
		if err != nil {
			return AcceptResult{}, fmt.Errorf("attribute push to MR board: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return AcceptResult{}, fmt.Errorf("commit accept: %w", err)
		}
		return AcceptResult{Accepted: true, Attributed: int(attributed)}, nil
	}

	// Lock the board row before mutating it: deliveries for one merge request
	// then serialize exactly like the delivery worker serializes posts.
	if err := q.StateEnsure(ctx, sqlcgen.StateEnsureParams{MrRef: ev.MRRef, UpdatedAt: now}); err != nil {
		return AcceptResult{}, fmt.Errorf("ensure board state for MR %s: %w", ev.MRRef, err)
	}
	old, err := q.StateGetForUpdate(ctx, ev.MRRef)
	if err != nil {
		return AcceptResult{}, fmt.Errorf("lock board state for MR %s: %w", ev.MRRef, err)
	}

	var changed bool
	switch ev.Kind {
	case event.KindMergeRequest:
		changed = mrStateChanged(old, ev)
		if err := q.StateApplyMR(ctx, sqlcgen.StateApplyMRParams{
			MrRef:             ev.MRRef,
			Title:             ev.Title,
			Author:            ev.Author,
			Url:               ev.URL,
			Iid:               ev.IID,
			SourceBranch:      ev.SourceBranch,
			TargetBranch:      ev.TargetBranch,
			SourceProjectID:   ev.SourceProjectID,
			Open:              ev.Open,
			ReviewersDone:     int32(ev.ReviewersDone),
			ReviewersRequired: int32(ev.ReviewersRequired),
			EditCount:         editDelta(ev),
			UpdatedAt:         now,
			ProjectPath:       ev.ProjectPath,
		}); err != nil {
			return AcceptResult{}, fmt.Errorf("apply merge request state for MR %s: %w", ev.MRRef, err)
		}
	case event.KindNote:
		if ev.NoteCreate && !ev.System {
			n, err := q.StateIncrComment(ctx, sqlcgen.StateIncrCommentParams{MrRef: ev.MRRef, UpdatedAt: now})
			if err != nil {
				return AcceptResult{}, fmt.Errorf("count comment for MR %s: %w", ev.MRRef, err)
			}
			changed = n > 0
		}
	case event.KindPipeline:
		n, err := q.StateSetPipeline(ctx, sqlcgen.StateSetPipelineParams{
			MrRef:          ev.MRRef,
			PipelineStatus: ev.PipelineStatus,
			UpdatedAt:      now,
		})
		if err != nil {
			return AcceptResult{}, fmt.Errorf("set pipeline state for MR %s: %w", ev.MRRef, err)
		}
		changed = n > 0
	case event.KindPush:
		// Handled above; keep the switch exhaustive.
		return AcceptResult{}, fmt.Errorf("push delivery reached merge request state path")
	default:
		return AcceptResult{}, fmt.Errorf("unprocessed event kind %q", ev.Kind)
	}

	ev.Plan.Refresh = changed || ev.Plan.Notice != ""
	if !planNeedsDelivery(ev.Plan) {
		if err := tx.Commit(); err != nil {
			return AcceptResult{}, fmt.Errorf("commit accept: %w", err)
		}
		return AcceptResult{Accepted: true}, nil
	}

	// Attach the attributed push count now that the row is locked and the
	// push delivery may already have arrived (SYSREQ-030 race).
	if ev.Plan.Notice == plan.Push {
		if old.LastPushCount > 0 {
			count := int(old.LastPushCount)
			ev.PushCount = &count
		}
	}

	payload, err := json.Marshal(ev)
	if err != nil {
		return AcceptResult{}, fmt.Errorf("encode event for project %q: %w", ev.ProjectPath, err)
	}
	if err := q.InsertOutbox(ctx, sqlcgen.InsertOutboxParams{
		MrRef:         ev.MRRef,
		Action:        ev.Action,
		DestChannel:   destChannel,
		Payload:       payload,
		NextAttemptAt: now,
	}); err != nil {
		return AcceptResult{}, fmt.Errorf("enqueue webhook %q: %w", ev.WebhookID, err)
	}
	if err := tx.Commit(); err != nil {
		return AcceptResult{}, fmt.Errorf("commit accept: %w", err)
	}
	return AcceptResult{Accepted: true, Enqueued: true}, nil
}

// planNeedsDelivery reports whether a plan maps to any Mattermost operation.
func planNeedsDelivery(p *event.Plan) bool {
	return p != nil && (p.CreateRoot || p.Notice != "" || p.Refresh)
}

// editDelta counts one edit per delivery that pushed or changed the
// description (SYSREQ-027).
func editDelta(ev *event.Event) int32 {
	if ev.EditChanged {
		return 1
	}
	return 0
}

// mrStateChanged reports whether an merge request delivery moves any field
// the board renders. Reviewer tallies are recomputed, never incremented, so
// approval resets self-heal here without any explicit decrement rule.
func mrStateChanged(old sqlcgen.StateGetForUpdateRow, ev *event.Event) bool {
	return ev.EditChanged ||
		ev.Title != old.Title ||
		ev.Author != old.Author ||
		ev.URL != old.Url ||
		ev.IID != old.Iid ||
		ev.SourceBranch != old.SourceBranch ||
		ev.TargetBranch != old.TargetBranch ||
		ev.SourceProjectID != old.SourceProjectID ||
		ev.Open != old.Open ||
		ev.ReviewersDone != int(old.ReviewersDone) ||
		ev.ReviewersRequired != int(old.ReviewersRequired)
}

// PruneSeen deletes seen records older than the retention horizon.
func (p *Postgres) PruneSeen(ctx context.Context, olderThan time.Time) error {
	if err := p.base.PruneSeen(ctx, olderThan); err != nil {
		return fmt.Errorf("prune seen webhooks: %w", err)
	}
	return nil
}

// ClaimDue takes the oldest due job under a lease, or nil when none is due.
// Arrival order plus the single-writer v1 deployment preserve one message per
// merge request at a time (SYSREQ-020 serialization).
func (p *Postgres) ClaimDue(ctx context.Context, now time.Time, lease time.Duration) (*Job, error) {
	row, err := p.base.ClaimDue(ctx, sqlcgen.ClaimDueParams{
		ClaimedAt:   sql.NullTime{Time: now, Valid: true},
		LeaseExpiry: sql.NullTime{Time: now.Add(lease), Valid: true},
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim due job: %w", err)
	}
	return &Job{
		ID:            row.ID,
		MRRef:         row.MrRef,
		Action:        row.Action,
		DestChannel:   row.DestChannel,
		Payload:       row.Payload,
		Attempts:      int(row.Attempts),
		NoticePosted:  row.NoticePosted,
		RootRefreshed: row.RootRefreshed,
	}, nil
}

// ReapLeases returns jobs whose worker died mid-claim to the pending pool.
func (p *Postgres) ReapLeases(ctx context.Context, now time.Time) error {
	if err := p.base.ReapLeases(ctx, sql.NullTime{Time: now, Valid: true}); err != nil {
		return fmt.Errorf("reap expired leases: %w", err)
	}
	return nil
}

// Settle applies a settlement to a claimed job.
func (p *Postgres) Settle(ctx context.Context, jobID int64, now time.Time, s Settlement) error {
	var err error
	switch s.Kind {
	case SettlementSent:
		err = p.base.SettleSent(ctx, sqlcgen.SettleSentParams{
			ID:        jobID,
			SettledAt: sql.NullTime{Time: now, Valid: true},
		})
	case SettlementSkipped:
		err = p.base.SettleSkip(ctx, sqlcgen.SettleSkipParams{
			ID:            jobID,
			SettledReason: sql.NullString{String: s.Reason, Valid: true},
			SettledAt:     sql.NullTime{Time: now, Valid: true},
		})
	case SettlementRetried:
		err = p.base.SettleRetry(ctx, sqlcgen.SettleRetryParams{
			ID:            jobID,
			Attempts:      int32(s.Attempts),
			NextAttemptAt: s.Next,
			LastError:     sql.NullString{String: s.Err, Valid: s.Err != ""},
		})
	case SettlementParked:
		err = p.base.SettlePark(ctx, sqlcgen.SettleParkParams{
			ID:            jobID,
			Attempts:      int32(s.Attempts),
			SettledReason: sql.NullString{String: s.Reason, Valid: true},
			SettledAt:     sql.NullTime{Time: now, Valid: true},
			LastError:     sql.NullString{String: s.Err, Valid: s.Err != ""},
		})
	default:
		return fmt.Errorf("unknown settlement kind %d for job %d", int(s.Kind), jobID)
	}
	if err != nil {
		return fmt.Errorf("settle job %d: %w", jobID, err)
	}
	return nil
}

// Thread returns the mapping for a merge request, or nil when none exists.
func (p *Postgres) Thread(ctx context.Context, mrRef string) (*Thread, error) {
	row, err := p.base.ThreadGet(ctx, mrRef)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read thread mapping for MR %s: %w", mrRef, err)
	}
	return &Thread{
		ChannelID:  row.ChannelID,
		RootPostID: row.RootPostID,
		Open:       row.State == "open",
	}, nil
}

// OpenThread records the root post mapping for a merge request.
func (p *Postgres) OpenThread(ctx context.Context, mrRef, channelID, rootPostID string, now time.Time) error {
	if err := p.base.ThreadOpen(ctx, sqlcgen.ThreadOpenParams{
		MrRef:      mrRef,
		ChannelID:  channelID,
		RootPostID: rootPostID,
		CreatedAt:  now,
	}); err != nil {
		return fmt.Errorf("open thread mapping for MR %s: %w", mrRef, err)
	}
	return nil
}

// CloseThread marks a mapping closed so later replies stop (SYSREQ-026).
func (p *Postgres) CloseThread(ctx context.Context, mrRef string, now time.Time) error {
	if err := p.base.ThreadClose(ctx, sqlcgen.ThreadCloseParams{MrRef: mrRef, UpdatedAt: now}); err != nil {
		return fmt.Errorf("close thread mapping for MR %s: %w", mrRef, err)
	}
	return nil
}
