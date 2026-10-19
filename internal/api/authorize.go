package api

import (
	"context"
	"net/http"
	"slices"
)

// Every decision about who may do what with teams, sources, sinks and
// routes lives here, so that the web interface and automation get the same
// answers (SYSREQ-039, ADR-034). Administrators may do everything. A
// manager reads their teams and what those own, and manages the routes of
// their teams. A regular does neither.

// visibleTo returns whose view of the routing configuration the caller has:
// zero for an administrator, who sees everything, or the caller's own id,
// which shows the teams they manage.
func visibleTo(ctx context.Context) int64 {
	person := caller(ctx)
	if person.Administrator {
		return 0
	}
	return person.ID
}

// managesTeam reports whether the caller is an administrator or one of the
// team's managers.
func (s *Service) managesTeam(ctx context.Context, teamID int64) (bool, error) {
	person := caller(ctx)
	if person.Administrator {
		return true, nil
	}
	return s.store.ManagesTeam(ctx, person.ID, teamID)
}

// mayReadTeam refuses a caller who neither administers the instance nor
// manages the team.
func (s *Service) mayReadTeam(ctx context.Context, teamID int64) error {
	manages, err := s.managesTeam(ctx, teamID)
	if err != nil {
		return err
	}
	if !manages {
		return newProblem(http.StatusForbidden, "only the team's operators and administrators may see it")
	}
	return nil
}

// mayManageRoutes refuses a caller who may not change the team's routes.
func (s *Service) mayManageRoutes(ctx context.Context, teamID int64) error {
	manages, err := s.managesTeam(ctx, teamID)
	if err != nil {
		return err
	}
	if !manages {
		return newProblem(http.StatusForbidden, "only the team's operators and administrators may change its routes")
	}
	return nil
}

// mayReadOwned refuses a caller who may see none of the teams that own a
// source or sink.
func (s *Service) mayReadOwned(ctx context.Context, teams []int64) error {
	person := caller(ctx)
	if person.Administrator {
		return nil
	}
	for _, teamID := range teams {
		manages, err := s.store.ManagesTeam(ctx, person.ID, teamID)
		if err != nil {
			return err
		}
		if manages {
			return nil
		}
	}
	return newProblem(http.StatusForbidden, "only the operators of a team that owns it and administrators may see it")
}

// ownedBy refuses a source or sink that the team does not own, naming the
// field that named it.
func ownedBy(teamID int64, teams []int64, field string) error {
	if slices.Contains(teams, teamID) {
		return nil
	}
	return fieldProblem(http.StatusForbidden, "a route connects only the team's own sources and sinks", field, "is not the team's")
}
