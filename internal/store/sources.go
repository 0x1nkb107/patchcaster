package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

// ErrDuplicate reports an item that would repeat another one, such as a
// second sink for the same channel.
var ErrDuplicate = errors.New("another item is the same")

// The PostgreSQL error code of a unique violation.
const uniqueViolation = "23505"

// uniqueProblem turns a unique violation into the field error its
// constraint names in fields, and passes any other error on.
func uniqueProblem(err error, fields map[string]*FieldError) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolation {
		return err
	}
	if field, ok := fields[pgErr.ConstraintName]; ok {
		return field
	}
	return ErrDuplicate
}

// The constraints that guard a source, by the field they concern.
var (
	sourceUniques = map[string]*FieldError{
		"sources_name_key": {Field: "name", Err: ErrNameTaken},
		"sources_connection_id_scope_kind_scope_path_key": {Field: "scope", Err: ErrDuplicate},
	}
	sourceReferences = map[string]*FieldError{
		"sources_connection_id_fkey": {Field: "connection_id", Err: ErrUnknownReference},
	}
	sinkUniques = map[string]*FieldError{
		"sinks_name_key":                     {Field: "name", Err: ErrNameTaken},
		"sinks_connection_id_channel_id_key": {Field: "channel_id", Err: ErrDuplicate},
	}
	sinkReferences = map[string]*FieldError{
		"sinks_connection_id_fkey": {Field: "connection_id", Err: ErrUnknownReference},
	}
)

// Source selects the repositories of one forge connection by scope
// (SYSREQ-044). ScopeKind is "instance", "group" or "repository", and
// ScopePath names the group or repository. Teams lists the ids of the teams
// that own it.
type Source struct {
	ID           int64
	ConnectionID int64
	Name         string
	ScopeKind    string
	ScopePath    string
	Teams        []int64
	Version      int64
}

// Sink names one channel or room of a chat connection (SYSREQ-045).
// ChannelName is how the platform named it at the last check.
type Sink struct {
	ID           int64
	ConnectionID int64
	Name         string
	ChannelID    string
	ChannelName  string
	Teams        []int64
	Version      int64
}

func sourceFrom(row sqlcgen.SourceGetRow) (*Source, error) {
	source := &Source{
		ID:           row.ID,
		ConnectionID: row.ConnectionID,
		Name:         row.Name,
		ScopeKind:    row.ScopeKind,
		ScopePath:    row.ScopePath,
		Version:      row.Version,
	}
	if err := json.Unmarshal(row.Teams, &source.Teams); err != nil {
		return nil, fmt.Errorf("decode teams of source %d: %w", row.ID, err)
	}
	return source, nil
}

func sinkFrom(row sqlcgen.SinkGetRow) (*Sink, error) {
	sink := &Sink{
		ID:           row.ID,
		ConnectionID: row.ConnectionID,
		Name:         row.Name,
		ChannelID:    row.ChannelID,
		ChannelName:  row.ChannelName,
		Version:      row.Version,
	}
	if err := json.Unmarshal(row.Teams, &sink.Teams); err != nil {
		return nil, fmt.Errorf("decode teams of sink %d: %w", row.ID, err)
	}
	return sink, nil
}

func sourceTarget(s *Source) map[string]any {
	return map[string]any{
		"connection_id": s.ConnectionID,
		"name":          s.Name,
		"scope_kind":    s.ScopeKind,
		"scope_path":    s.ScopePath,
	}
}

func sinkTarget(s *Sink) map[string]any {
	return map[string]any{
		"connection_id": s.ConnectionID,
		"name":          s.Name,
		"channel_id":    s.ChannelID,
		"channel_name":  s.ChannelName,
	}
}

func readSource(ctx context.Context, q *sqlcgen.Queries, id int64) (*Source, error) {
	row, err := q.SourceGet(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read source %d: %w", id, err)
	}
	return sourceFrom(row)
}

func readSink(ctx context.Context, q *sqlcgen.Queries, id int64) (*Sink, error) {
	row, err := q.SinkGet(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read sink %d: %w", id, err)
	}
	return sinkFrom(row)
}

// CreateSource stores a new source, owned by no team yet.
func (p *Postgres) CreateSource(ctx context.Context, actor Actor, s Source, now time.Time) (*Source, error) {
	var created *Source
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		id, err := q.SourceCreate(ctx, sqlcgen.SourceCreateParams{
			ConnectionID: s.ConnectionID,
			Name:         s.Name,
			ScopeKind:    s.ScopeKind,
			ScopePath:    s.ScopePath,
			CreatedAt:    now,
		})
		if err != nil {
			err = uniqueProblem(err, sourceUniques)
			return referenceProblem(err, sourceReferences)
		}
		created, err = readSource(ctx, q, id)
		if err != nil {
			return err
		}
		return audit(ctx, q, actor, now, change{
			action:     "source.create",
			targetKind: "source",
			targetID:   itemID(id),
			after:      sourceTarget(created),
		})
	})
	return created, err
}

// Source reads one source, or returns ErrNotFound.
func (p *Postgres) Source(ctx context.Context, id int64) (*Source, error) {
	return readSource(ctx, p.base, id)
}

// Sources pages through the sources of the teams the person with visibleTo
// manages, or through all sources when visibleTo is zero.
func (p *Postgres) Sources(ctx context.Context, visibleTo, afterID int64, limit int) ([]Source, error) {
	rows, err := p.base.SourceList(ctx, sqlcgen.SourceListParams{
		VisibleTo: visibleTo,
		AfterID:   afterID,
		PageSize:  int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	sources := make([]Source, 0, len(rows))
	for _, row := range rows {
		source, err := sourceFrom(sqlcgen.SourceGetRow(row))
		if err != nil {
			return nil, err
		}
		sources = append(sources, *source)
	}
	return sources, nil
}

// UpdateSource changes a source's name and scope at the version in s, the
// one the caller read. Its connection stays.
func (p *Postgres) UpdateSource(ctx context.Context, actor Actor, s Source, now time.Time) (*Source, error) {
	var updated *Source
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		before, err := readSource(ctx, q, s.ID)
		if err != nil {
			return err
		}
		n, err := q.SourceUpdate(ctx, sqlcgen.SourceUpdateParams{
			ID:        s.ID,
			Name:      s.Name,
			ScopeKind: s.ScopeKind,
			ScopePath: s.ScopePath,
			UpdatedAt: now,
			Version:   s.Version,
		})
		if err != nil {
			return uniqueProblem(err, sourceUniques)
		}
		if n == 0 {
			return ErrVersionMismatch
		}
		updated, err = readSource(ctx, q, s.ID)
		if err != nil {
			return err
		}
		return audit(ctx, q, actor, now, change{
			action:     "source.update",
			targetKind: "source",
			targetID:   itemID(s.ID),
			before:     sourceTarget(before),
			after:      sourceTarget(updated),
		})
	})
	return updated, err
}

// DeleteSource removes a source at the version the caller read. A source
// that a route uses cannot go.
func (p *Postgres) DeleteSource(ctx context.Context, actor Actor, id, version int64, now time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		before, err := readSource(ctx, q, id)
		if err != nil {
			return err
		}
		n, err := q.SourceDelete(ctx, sqlcgen.SourceDeleteParams{ID: id, Version: version})
		if err != nil {
			return referenceProblem(fmt.Errorf("delete source %d: %w", id, err), nil)
		}
		if n == 0 {
			return ErrVersionMismatch
		}
		return audit(ctx, q, actor, now, change{
			action:     "source.delete",
			targetKind: "source",
			targetID:   itemID(id),
			before:     sourceTarget(before),
		})
	})
}

// CreateSink stores a new sink, owned by no team yet.
func (p *Postgres) CreateSink(ctx context.Context, actor Actor, s Sink, now time.Time) (*Sink, error) {
	var created *Sink
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		id, err := q.SinkCreate(ctx, sqlcgen.SinkCreateParams{
			ConnectionID: s.ConnectionID,
			Name:         s.Name,
			ChannelID:    s.ChannelID,
			ChannelName:  s.ChannelName,
			CreatedAt:    now,
		})
		if err != nil {
			err = uniqueProblem(err, sinkUniques)
			return referenceProblem(err, sinkReferences)
		}
		created, err = readSink(ctx, q, id)
		if err != nil {
			return err
		}
		return audit(ctx, q, actor, now, change{
			action:     "sink.create",
			targetKind: "sink",
			targetID:   itemID(id),
			after:      sinkTarget(created),
		})
	})
	return created, err
}

// Sink reads one sink, or returns ErrNotFound.
func (p *Postgres) Sink(ctx context.Context, id int64) (*Sink, error) {
	return readSink(ctx, p.base, id)
}

// Sinks pages through the sinks of the teams the person with visibleTo
// manages, or through all sinks when visibleTo is zero.
func (p *Postgres) Sinks(ctx context.Context, visibleTo, afterID int64, limit int) ([]Sink, error) {
	rows, err := p.base.SinkList(ctx, sqlcgen.SinkListParams{
		VisibleTo: visibleTo,
		AfterID:   afterID,
		PageSize:  int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list sinks: %w", err)
	}
	sinks := make([]Sink, 0, len(rows))
	for _, row := range rows {
		sink, err := sinkFrom(sqlcgen.SinkGetRow(row))
		if err != nil {
			return nil, err
		}
		sinks = append(sinks, *sink)
	}
	return sinks, nil
}

// UpdateSink changes a sink's name and channel at the version in s, the one
// the caller read. Its connection stays.
func (p *Postgres) UpdateSink(ctx context.Context, actor Actor, s Sink, now time.Time) (*Sink, error) {
	var updated *Sink
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		before, err := readSink(ctx, q, s.ID)
		if err != nil {
			return err
		}
		n, err := q.SinkUpdate(ctx, sqlcgen.SinkUpdateParams{
			ID:          s.ID,
			Name:        s.Name,
			ChannelID:   s.ChannelID,
			ChannelName: s.ChannelName,
			UpdatedAt:   now,
			Version:     s.Version,
		})
		if err != nil {
			return uniqueProblem(err, sinkUniques)
		}
		if n == 0 {
			return ErrVersionMismatch
		}
		updated, err = readSink(ctx, q, s.ID)
		if err != nil {
			return err
		}
		return audit(ctx, q, actor, now, change{
			action:     "sink.update",
			targetKind: "sink",
			targetID:   itemID(s.ID),
			before:     sinkTarget(before),
			after:      sinkTarget(updated),
		})
	})
	return updated, err
}

// DeleteSink removes a sink at the version the caller read. A sink that a
// route uses cannot go.
func (p *Postgres) DeleteSink(ctx context.Context, actor Actor, id, version int64, now time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		before, err := readSink(ctx, q, id)
		if err != nil {
			return err
		}
		n, err := q.SinkDelete(ctx, sqlcgen.SinkDeleteParams{ID: id, Version: version})
		if err != nil {
			return referenceProblem(fmt.Errorf("delete sink %d: %w", id, err), nil)
		}
		if n == 0 {
			return ErrVersionMismatch
		}
		return audit(ctx, q, actor, now, change{
			action:     "sink.delete",
			targetKind: "sink",
			targetID:   itemID(id),
			before:     sinkTarget(before),
		})
	})
}
