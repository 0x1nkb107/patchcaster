package api

import (
	"context"
	"net/http"
	"slices"
)

// Every decision about who may do what with teams, sources, sinks and
// routes lives here, so that the web interface and automation get the same
// answers (SYSREQ-039, ADR-034). Administrators may do everything. An
// operator reads their teams and what those own, and manages the routes of
// their teams. A user manages neither.

// visibleTo returns whose view of the routing configuration the caller has:
// zero for an administrator, who sees everything, or the caller's own id,
// which shows the teams they operate.
func visibleTo(ctx context.Context) int64 {
	person := caller(ctx)
	if person.Administrator {
		return 0
	}
	return person.ID
}

// operatesTeam reports whether the caller is an administrator or one of the
// team's operators.
func (s *Service) operatesTeam(ctx context.Context, teamID int64) (bool, error) {
	person := caller(ctx)
	if person.Administrator {
		return true, nil
	}
	return s.store.OperatesTeam(ctx, person.ID, teamID)
}

// mayReadTeam refuses a caller who neither administers the instance nor
// operates the team.
func (s *Service) mayReadTeam(ctx context.Context, teamID int64) error {
	operates, err := s.operatesTeam(ctx, teamID)
	if err != nil {
		return err
	}
	if !operates {
		return newProblem(http.StatusForbidden, "only the team's operators and administrators may see it")
	}
	return nil
}

// mayManageRoutes refuses a caller who may not change the team's routes.
func (s *Service) mayManageRoutes(ctx context.Context, teamID int64) error {
	operates, err := s.operatesTeam(ctx, teamID)
	if err != nil {
		return err
	}
	if !operates {
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
		operates, err := s.store.OperatesTeam(ctx, person.ID, teamID)
		if err != nil {
			return err
		}
		if operates {
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
