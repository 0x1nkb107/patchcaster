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

// ErrNotTheTeams reports a source or sink that the route's team does not
// own (SYSREQ-046).
var ErrNotTheTeams = errors.New("the team does not own the item")

// The constraints that guard a route, by the field they concern.
var (
	routeReferences = map[string]*FieldError{
		"routes_team_id_fkey":           {Field: "team_id", Err: ErrUnknownReference},
		"routes_team_id_source_id_fkey": {Field: "source_id", Err: ErrNotTheTeams},
		"routes_team_id_sink_id_fkey":   {Field: "sink_id", Err: ErrNotTheTeams},
	}
	routeUniques = map[string]*FieldError{
		"routes_team_id_source_id_sink_id_key": {Field: "sink_id", Err: ErrDuplicate},
	}
)

// Route forwards a team's source into one of its sinks (SYSREQ-046).
// MutedReplies names the reply kinds it does not post. The board updates
// whatever they are (SYSREQ-047).
type Route struct {
	ID           int64
	TeamID       int64
	SourceID     int64
	SinkID       int64
	MutedReplies []string
	Version      int64
}

func routeFrom(row sqlcgen.RouteGetRow) (*Route, error) {
	route := &Route{
		ID:       row.ID,
		TeamID:   row.TeamID,
		SourceID: row.SourceID,
		SinkID:   row.SinkID,
		Version:  row.Version,
	}
	if err := json.Unmarshal(row.MutedReplies, &route.MutedReplies); err != nil {
		return nil, fmt.Errorf("decode muted replies of route %d: %w", row.ID, err)
	}
	return route, nil
}

func routeTarget(r *Route) map[string]any {
	return map[string]any{
		"team_id":       r.TeamID,
		"source_id":     r.SourceID,
		"sink_id":       r.SinkID,
		"muted_replies": replyList(r.MutedReplies),
	}
}

// replyList returns reply kinds as a list that encodes as a JSON array,
// never as null.
func replyList(kinds []string) []string {
	if kinds == nil {
		return []string{}
	}
	return kinds
}

func encodeReplies(kinds []string) json.RawMessage {
	encoded, _ := json.Marshal(replyList(kinds)) // a list of strings always encodes
	return encoded
}

func readRoute(ctx context.Context, q *sqlcgen.Queries, id int64) (*Route, error) {
	row, err := q.RouteGet(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read route %d: %w", id, err)
	}
	return routeFrom(row)
}

// CreateRoute stores a new route. The database refuses a source or sink the
// team does not own with ErrNotTheTeams.
func (p *Postgres) CreateRoute(ctx context.Context, actor Actor, r Route, now time.Time) (*Route, error) {
	var created *Route
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		row, err := q.RouteCreate(ctx, sqlcgen.RouteCreateParams{
			TeamID:       r.TeamID,
			SourceID:     r.SourceID,
			SinkID:       r.SinkID,
			MutedReplies: encodeReplies(r.MutedReplies),
			CreatedAt:    now,
		})
		if err != nil {
			err = uniqueProblem(err, routeUniques)
			return referenceProblem(err, routeReferences)
		}
		created, err = routeFrom(sqlcgen.RouteGetRow(row))
		if err != nil {
			return err
		}
		return audit(ctx, q, actor, now, change{
			action:     "route.create",
			targetKind: "route",
			targetID:   itemID(created.ID),
			after:      routeTarget(created),
		})
	})
	return created, err
}

// Route reads one route, or returns ErrNotFound.
func (p *Postgres) Route(ctx context.Context, id int64) (*Route, error) {
	return readRoute(ctx, p.base, id)
}

// Routes pages through the routes of the teams the person with visibleTo
// operates, or through all routes when visibleTo is zero. A teamID other
// than zero narrows the list to that team.
func (p *Postgres) Routes(ctx context.Context, visibleTo, teamID, afterID int64, limit int) ([]Route, error) {
	rows, err := p.base.RouteList(ctx, sqlcgen.RouteListParams{
		VisibleTo: visibleTo,
		TeamID:    teamID,
		AfterID:   afterID,
		PageSize:  int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	routes := make([]Route, 0, len(rows))
	for _, row := range rows {
		route, err := routeFrom(sqlcgen.RouteGetRow(row))
		if err != nil {
			return nil, err
		}
		routes = append(routes, *route)
	}
	return routes, nil
}

// UpdateRoute changes a route's muted replies at the version in r, the one
// the caller read. Its team, source and sink stay.
func (p *Postgres) UpdateRoute(ctx context.Context, actor Actor, r Route, now time.Time) (*Route, error) {
	var updated *Route
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		before, err := readRoute(ctx, q, r.ID)
		if err != nil {
			return err
		}
		n, err := q.RouteUpdate(ctx, sqlcgen.RouteUpdateParams{
			ID:           r.ID,
			MutedReplies: encodeReplies(r.MutedReplies),
			UpdatedAt:    now,
			Version:      r.Version,
		})
		if err != nil {
			return fmt.Errorf("update route %d: %w", r.ID, err)
		}
		if n == 0 {
			return ErrVersionMismatch
		}
		updated, err = readRoute(ctx, q, r.ID)
		if err != nil {
			return err
		}
		return audit(ctx, q, actor, now, change{
			action:     "route.update",
			targetKind: "route",
			targetID:   itemID(r.ID),
			before:     routeTarget(before),
			after:      routeTarget(updated),
		})
	})
	return updated, err
}

// DeleteRoute removes a route at the version the caller read. Messages it
// queued keep the destination they were accepted with (SCEN-019).
func (p *Postgres) DeleteRoute(ctx context.Context, actor Actor, id, version int64, now time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		before, err := readRoute(ctx, q, id)
		if err != nil {
			return err
		}
		n, err := q.RouteDelete(ctx, sqlcgen.RouteDeleteParams{ID: id, Version: version})
		if err != nil {
			return fmt.Errorf("delete route %d: %w", id, err)
		}
		if n == 0 {
			return ErrVersionMismatch
		}
		return audit(ctx, q, actor, now, change{
			action:     "route.delete",
			targetKind: "route",
			targetID:   itemID(id),
			before:     routeTarget(before),
		})
	})
}
