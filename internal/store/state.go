package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

// State is the merge request board snapshot (SYSREQ-027): everything a root
// refresh renders without consulting GitLab again.
type State struct {
	MrRef             string
	Title             string
	Author            string
	URL               string
	IID               int64
	SourceBranch      string
	TargetBranch      string
	ProjectPath       string
	SourceProjectID   int64
	Open              bool
	PipelineStatus    string // "" until the first linked pipeline event: no segment
	Comments          int
	Edits             int
	ReviewersDone     int
	ReviewersRequired int
	LastPushCount     int // 0 = no attributed push: push notices omit the count (SYSREQ-030)
}

// State reads the board for a merge request, or nil when none exists.
func (p *Postgres) State(ctx context.Context, mrRef string) (*State, error) {
	row, err := p.base.StateGet(ctx, mrRef)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read board state for MR %s: %w", mrRef, err)
	}
	return &State{
		MrRef:             row.MrRef,
		Title:             row.Title,
		Author:            row.Author,
		URL:               row.Url,
		IID:               row.Iid,
		SourceBranch:      row.SourceBranch,
		TargetBranch:      row.TargetBranch,
		ProjectPath:       row.ProjectPath,
		SourceProjectID:   row.SourceProjectID,
		Open:              row.Open,
		PipelineStatus:    row.PipelineStatus,
		Comments:          int(row.CommentCount),
		Edits:             int(row.EditCount),
		ReviewersDone:     int(row.ReviewersDone),
		ReviewersRequired: int(row.ReviewersRequired),
		LastPushCount:     int(row.LastPushCount),
	}, nil
}

// CloseState marks the merge request no longer open, stopping push
// attribution (SYSREQ-030). Called when the delivery flow closes the thread.
func (p *Postgres) CloseState(ctx context.Context, mrRef string, now time.Time) error {
	if err := p.base.StateClose(ctx, sqlcgen.StateCloseParams{MrRef: mrRef, UpdatedAt: now}); err != nil {
		return fmt.Errorf("close board state for MR %s: %w", mrRef, err)
	}
	return nil
}

// MarkNoticePosted records that a delivery's notice reply reached Mattermost,
// so a retry of the same delivery skips that phase (SYSREQ-028, ADR-014).
func (p *Postgres) MarkNoticePosted(ctx context.Context, jobID int64, now time.Time) error {
	if err := p.base.MarkNoticePosted(ctx, sqlcgen.MarkNoticePostedParams{ID: jobID, UpdatedAt: now}); err != nil {
		return fmt.Errorf("mark notice posted on job %d: %w", jobID, err)
	}
	return nil
}

// MarkRootRefreshed records that a delivery's root status update reached
// Mattermost, so a retry skips that phase.
func (p *Postgres) MarkRootRefreshed(ctx context.Context, jobID int64, now time.Time) error {
	if err := p.base.MarkRootRefreshed(ctx, sqlcgen.MarkRootRefreshedParams{ID: jobID, UpdatedAt: now}); err != nil {
		return fmt.Errorf("mark root refreshed on job %d: %w", jobID, err)
	}
	return nil
}
