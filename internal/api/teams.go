package api

import (
	"context"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// ListTeams pages through the teams the caller may see.
func (s *Service) ListTeams(ctx context.Context, params v1gen.ListTeamsParams) (*v1gen.TeamPage, error) {
	afterID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, err
	}
	limit := params.Limit.Or(defaultPageSize)
	teams, err := s.store.Teams(ctx, visibleTo(ctx), afterID, limit+1)
	if err != nil {
		return nil, err
	}
	teams, next := pageOf(teams, limit, func(t store.Team) int64 { return t.ID })
	items := make([]v1gen.Team, 0, len(teams))
	for i := range teams {
		items = append(items, teamOf(&teams[i]))
	}
	return &v1gen.TeamPage{Items: items, NextCursor: next}, nil
}

// CreateTeam creates a team without managers, sources or sinks, for
// administrators.
func (s *Service) CreateTeam(ctx context.Context, req *v1gen.TeamRequest) (*v1gen.Team, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	created, err := s.store.CreateTeam(ctx, actorOf(ctx), req.Name, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	team := teamOf(created)
	return &team, nil
}

// GetTeam reads a team the caller may see.
func (s *Service) GetTeam(ctx context.Context, params v1gen.GetTeamParams) (*v1gen.TeamHeaders, error) {
	team, err := s.store.Team(ctx, params.ID)
	if err != nil {
		return nil, storeProblem(err)
	}
	if err := s.mayReadTeam(ctx, team.ID); err != nil {
		return nil, err
	}
	return teamHeaders(team), nil
}

// UpdateTeam renames a team and sets its managers, sources and sinks, for
// administrators.
func (s *Service) UpdateTeam(ctx context.Context, req *v1gen.TeamUpdate, params v1gen.UpdateTeamParams) (*v1gen.TeamHeaders, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return nil, err
	}
	updated, err := s.store.UpdateTeam(ctx, actorOf(ctx), store.Team{
		ID:       params.ID,
		Name:     req.Name,
		Managers: req.Managers,
		Sources:  req.Sources,
		Sinks:    req.Sinks,
		Version:  version,
	}, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	return teamHeaders(updated), nil
}

// DeleteTeam removes a team without routes, for administrators.
func (s *Service) DeleteTeam(ctx context.Context, params v1gen.DeleteTeamParams) error {
	if _, err := administrator(ctx); err != nil {
		return err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return err
	}
	if err := s.store.DeleteTeam(ctx, actorOf(ctx), params.ID, version, s.now()); err != nil {
		return storeProblem(err)
	}
	return nil
}

func teamOf(t *store.Team) v1gen.Team {
	return v1gen.Team{
		ID:       t.ID,
		Name:     t.Name,
		Managers: nonNilIDs(t.Managers),
		Sources:  nonNilIDs(t.Sources),
		Sinks:    nonNilIDs(t.Sinks),
		Etag:     etagOf(t.Version),
	}
}

func teamHeaders(t *store.Team) *v1gen.TeamHeaders {
	return &v1gen.TeamHeaders{
		ETag:     v1gen.NewOptString(etagOf(t.Version)),
		Response: teamOf(t),
	}
}

// nonNilIDs returns ids as a list that encodes as a JSON array, never as
// null.
func nonNilIDs(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}
