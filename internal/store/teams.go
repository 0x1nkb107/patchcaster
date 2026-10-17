package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

var (
	// ErrInUse reports an item that others still refer to, such as a
	// source a route uses or a team with routes.
	ErrInUse = errors.New("the item is in use")
	// ErrUnknownReference reports an id that names no item.
	ErrUnknownReference = errors.New("the id names no item")
)

// FieldError ties a store error to the field of the change that caused it,
// so that the API can name the field.
type FieldError struct {
	Field string
	Err   error
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Err.Error() }
func (e *FieldError) Unwrap() error { return e.Err }

// The PostgreSQL error codes the store turns into its own errors.
const foreignKeyViolation = "23503"

// referenceProblem turns a foreign key violation into the store's error. A
// constraint in fields names the field at fault, and whether the violation
// means an unknown id or an item still in use. Any other violation means the
// item is in use.
func referenceProblem(err error, fields map[string]*FieldError) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != foreignKeyViolation {
		return err
	}
	if field, ok := fields[pgErr.ConstraintName]; ok {
		return field
	}
	return ErrInUse
}

// teamReferences names the fields of a team change that its constraints
// guard.
var teamReferences = map[string]*FieldError{
	"team_operators_person_id_fkey": {Field: "operators", Err: ErrUnknownReference},
	"team_sources_source_id_fkey":   {Field: "sources", Err: ErrUnknownReference},
	"team_sinks_sink_id_fkey":       {Field: "sinks", Err: ErrUnknownReference},
	"routes_team_id_source_id_fkey": {Field: "sources", Err: ErrInUse},
	"routes_team_id_sink_id_fkey":   {Field: "sinks", Err: ErrInUse},
}

// Team scopes what its operators may route (ADR-034): the people who
// operate it, and the sources and sinks it owns, by their ids.
type Team struct {
	ID        int64
	Name      string
	Operators []int64
	Sources   []int64
	Sinks     []int64
	Version   int64
}

// teamFrom reads a team row. Every team query returns the same columns, so
// their row types convert to this one.
func teamFrom(row sqlcgen.TeamGetRow) (*Team, error) {
	team := &Team{ID: row.ID, Name: row.Name, Version: row.Version}
	for _, list := range []struct {
		into *[]int64
		raw  json.RawMessage
	}{{&team.Operators, row.Operators}, {&team.Sources, row.Sources}, {&team.Sinks, row.Sinks}} {
		if err := json.Unmarshal(list.raw, list.into); err != nil {
			return nil, fmt.Errorf("decode members of team %d: %w", row.ID, err)
		}
	}
	return team, nil
}

// teamTarget is how the audit log shows a team.
func teamTarget(t *Team) map[string]any {
	return map[string]any{
		"name":      t.Name,
		"operators": idList(t.Operators),
		"sources":   idList(t.Sources),
		"sinks":     idList(t.Sinks),
	}
}

// idList returns ids as a list that encodes as a JSON array, never as null.
func idList(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

// encodeIDs encodes ids for the queries that take them as a JSON array.
func encodeIDs(ids []int64) json.RawMessage {
	encoded, _ := json.Marshal(idList(ids)) // a list of integers always encodes
	return encoded
}

func itemID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// readTeam reads a team in the transaction q belongs to.
func readTeam(ctx context.Context, q *sqlcgen.Queries, id int64) (*Team, error) {
	row, err := q.TeamGet(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read team %d: %w", id, err)
	}
	return teamFrom(row)
}

// CreateTeam stores a new team without operators, sources or sinks.
func (p *Postgres) CreateTeam(ctx context.Context, actor Actor, name string, now time.Time) (*Team, error) {
	var created *Team
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		id, err := q.TeamCreate(ctx, sqlcgen.TeamCreateParams{Name: name, CreatedAt: now})
		if err != nil {
			return nameTaken(fmt.Errorf("store team: %w", err))
		}
		created, err = readTeam(ctx, q, id)
		if err != nil {
			return err
		}
		return audit(ctx, q, actor, now, change{
			action:     "team.create",
			targetKind: "team",
			targetID:   itemID(id),
			after:      teamTarget(created),
		})
	})
	return created, err
}

// Team reads one team, or returns ErrNotFound.
func (p *Postgres) Team(ctx context.Context, id int64) (*Team, error) {
	return readTeam(ctx, p.base, id)
}

// Teams pages through the teams the person with visibleTo operates, or
// through all teams when visibleTo is zero, as for an administrator.
func (p *Postgres) Teams(ctx context.Context, visibleTo, afterID int64, limit int) ([]Team, error) {
	rows, err := p.base.TeamList(ctx, sqlcgen.TeamListParams{
		VisibleTo: visibleTo,
		AfterID:   afterID,
		PageSize:  int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	teams := make([]Team, 0, len(rows))
	for _, row := range rows {
		team, err := teamFrom(sqlcgen.TeamGetRow(row))
		if err != nil {
			return nil, err
		}
		teams = append(teams, *team)
	}
	return teams, nil
}

// UpdateTeam renames a team and sets its operators, sources and sinks, at
// the version in t, the one the caller read. A source or sink that one of
// the team's routes uses cannot leave the team.
func (p *Postgres) UpdateTeam(ctx context.Context, actor Actor, t Team, now time.Time) (*Team, error) {
	var updated *Team
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		before, err := readTeam(ctx, q, t.ID)
		if err != nil {
			return err
		}
		n, err := q.TeamUpdate(ctx, sqlcgen.TeamUpdateParams{
			ID:        t.ID,
			Name:      t.Name,
			UpdatedAt: now,
			Version:   t.Version,
		})
		if err != nil {
			return nameTaken(fmt.Errorf("update team %d: %w", t.ID, err))
		}
		if n == 0 {
			return ErrVersionMismatch
		}
		if err := setTeamMembers(ctx, q, t); err != nil {
			return referenceProblem(err, teamReferences)
		}
		updated, err = readTeam(ctx, q, t.ID)
		if err != nil {
			return err
		}
		return audit(ctx, q, actor, now, change{
			action:     "team.update",
			targetKind: "team",
			targetID:   itemID(t.ID),
			before:     teamTarget(before),
			after:      teamTarget(updated),
		})
	})
	return updated, err
}

// setTeamMembers makes the team's operators, sources and sinks the ones in
// t: it removes the others first, then adds the new ones.
func setTeamMembers(ctx context.Context, q *sqlcgen.Queries, t Team) error {
	operators, sources, sinks := encodeIDs(t.Operators), encodeIDs(t.Sources), encodeIDs(t.Sinks)
	if err := q.TeamOperatorsKeep(ctx, sqlcgen.TeamOperatorsKeepParams{TeamID: t.ID, Keep: operators}); err != nil {
		return fmt.Errorf("remove operators of team %d: %w", t.ID, err)
	}
	if err := q.TeamOperatorsAdd(ctx, sqlcgen.TeamOperatorsAddParams{TeamID: t.ID, Add: operators}); err != nil {
		return fmt.Errorf("add operators to team %d: %w", t.ID, err)
	}
	if err := q.TeamSourcesKeep(ctx, sqlcgen.TeamSourcesKeepParams{TeamID: t.ID, Keep: sources}); err != nil {
		return fmt.Errorf("remove sources of team %d: %w", t.ID, err)
	}
	if err := q.TeamSourcesAdd(ctx, sqlcgen.TeamSourcesAddParams{TeamID: t.ID, Add: sources}); err != nil {
		return fmt.Errorf("add sources to team %d: %w", t.ID, err)
	}
	if err := q.TeamSinksKeep(ctx, sqlcgen.TeamSinksKeepParams{TeamID: t.ID, Keep: sinks}); err != nil {
		return fmt.Errorf("remove sinks of team %d: %w", t.ID, err)
	}
	if err := q.TeamSinksAdd(ctx, sqlcgen.TeamSinksAddParams{TeamID: t.ID, Add: sinks}); err != nil {
		return fmt.Errorf("add sinks to team %d: %w", t.ID, err)
	}
	return nil
}

// DeleteTeam removes a team at the version the caller read. A team with
// routes cannot go: its routes must be removed first.
func (p *Postgres) DeleteTeam(ctx context.Context, actor Actor, id, version int64, now time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		before, err := readTeam(ctx, q, id)
		if err != nil {
			return err
		}
		n, err := q.TeamDelete(ctx, sqlcgen.TeamDeleteParams{ID: id, Version: version})
		if err != nil {
			return referenceProblem(fmt.Errorf("delete team %d: %w", id, err), nil)
		}
		if n == 0 {
			return ErrVersionMismatch
		}
		return audit(ctx, q, actor, now, change{
			action:     "team.delete",
			targetKind: "team",
			targetID:   itemID(id),
			before:     teamTarget(before),
		})
	})
}

// OperatesTeam reports whether a person operates a team (SYSREQ-039).
func (p *Postgres) OperatesTeam(ctx context.Context, personID, teamID int64) (bool, error) {
	operates, err := p.base.TeamOperatedBy(ctx, sqlcgen.TeamOperatedByParams{TeamID: teamID, PersonID: personID})
	if err != nil {
		return false, fmt.Errorf("check operator of team %d: %w", teamID, err)
	}
	return operates, nil
}
