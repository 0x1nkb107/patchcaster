package api

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// replyKinds lists the reply kinds a route may mute, in the order the
// description lists them (SYSREQ-047).
var replyKinds = []v1gen.ReplyKind{
	v1gen.ReplyKindComment,
	v1gen.ReplyKindReview,
	v1gen.ReplyKindPipeline,
	v1gen.ReplyKindPush,
	v1gen.ReplyKindMerge,
	v1gen.ReplyKindClose,
}

// ListRoutes pages through the routes the caller may see, of one team when
// the request names it.
func (s *Service) ListRoutes(ctx context.Context, params v1gen.ListRoutesParams) (*v1gen.RoutePage, error) {
	teamID, narrowed := params.TeamID.Get()
	if narrowed {
		if err := s.mayReadTeam(ctx, teamID); err != nil {
			return nil, err
		}
	}
	afterID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, err
	}
	limit := params.Limit.Or(defaultPageSize)
	routes, err := s.store.Routes(ctx, visibleTo(ctx), teamID, afterID, limit+1)
	if err != nil {
		return nil, err
	}
	routes, next := pageOf(routes, limit, func(r store.Route) int64 { return r.ID })
	items := make([]v1gen.TeamRoute, 0, len(routes))
	for i := range routes {
		items = append(items, routeOf(&routes[i]))
	}
	return &v1gen.RoutePage{Items: items, NextCursor: next}, nil
}

// CreateRoute routes a team's source into one of its sinks, for the team's
// managers and administrators (SCEN-018).
func (s *Service) CreateRoute(ctx context.Context, req *v1gen.RouteRequest) (*v1gen.TeamRoute, error) {
	if err := s.mayManageRoutes(ctx, req.TeamID); err != nil {
		return nil, err
	}
	if err := s.teamOwns(ctx, req.TeamID, req.SourceID, req.SinkID); err != nil {
		return nil, err
	}
	created, err := s.store.CreateRoute(ctx, actorOf(ctx), store.Route{
		TeamID:       req.TeamID,
		SourceID:     req.SourceID,
		SinkID:       req.SinkID,
		MutedReplies: mutedReplies(req.MutedReplies),
	}, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	route := routeOf(created)
	return &route, nil
}

// teamOwns refuses a source or sink that the team does not own, with 403,
// and one that does not exist, with 422. The database refuses the same
// once more, should the team change meanwhile.
func (s *Service) teamOwns(ctx context.Context, teamID, sourceID, sinkID int64) error {
	source, err := s.store.Source(ctx, sourceID)
	if errors.Is(err, store.ErrNotFound) {
		return fieldProblem(http.StatusUnprocessableEntity, "the source does not exist", "source_id", "names no source")
	}
	if err != nil {
		return err
	}
	if err := ownedBy(teamID, source.Teams, "source_id"); err != nil {
		return err
	}
	sink, err := s.store.Sink(ctx, sinkID)
	if errors.Is(err, store.ErrNotFound) {
		return fieldProblem(http.StatusUnprocessableEntity, "the sink does not exist", "sink_id", "names no sink")
	}
	if err != nil {
		return err
	}
	return ownedBy(teamID, sink.Teams, "sink_id")
}

// GetRoute reads a route the caller may see.
func (s *Service) GetRoute(ctx context.Context, params v1gen.GetRouteParams) (*v1gen.TeamRouteHeaders, error) {
	route, err := s.store.Route(ctx, params.ID)
	if err != nil {
		return nil, storeProblem(err)
	}
	if err := s.mayReadTeam(ctx, route.TeamID); err != nil {
		return nil, err
	}
	return routeHeaders(route), nil
}

// UpdateRoute changes which replies a route mutes, for the team's managers
// and administrators.
func (s *Service) UpdateRoute(ctx context.Context, req *v1gen.RouteUpdate, params v1gen.UpdateRouteParams) (*v1gen.TeamRouteHeaders, error) {
	current, err := s.manageableRoute(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return nil, err
	}
	updated, err := s.store.UpdateRoute(ctx, actorOf(ctx), store.Route{
		ID:           current.ID,
		MutedReplies: mutedReplies(req.MutedReplies),
		Version:      version,
	}, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	return routeHeaders(updated), nil
}

// DeleteRoute removes a route, for the team's managers and administrators.
func (s *Service) DeleteRoute(ctx context.Context, params v1gen.DeleteRouteParams) error {
	current, err := s.manageableRoute(ctx, params.ID)
	if err != nil {
		return err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return err
	}
	if err := s.store.DeleteRoute(ctx, actorOf(ctx), current.ID, version, s.now()); err != nil {
		return storeProblem(err)
	}
	return nil
}

// manageableRoute reads a route whose team the caller may manage.
func (s *Service) manageableRoute(ctx context.Context, id int64) (*store.Route, error) {
	route, err := s.store.Route(ctx, id)
	if err != nil {
		return nil, storeProblem(err)
	}
	if err := s.mayManageRoutes(ctx, route.TeamID); err != nil {
		return nil, err
	}
	return route, nil
}

// mutedReplies keeps each muted reply kind once, in the description's
// order, so that equal filters are stored alike.
func mutedReplies(given []v1gen.ReplyKind) []string {
	muted := []string{}
	for _, kind := range replyKinds {
		if slices.Contains(given, kind) {
			muted = append(muted, string(kind))
		}
	}
	return muted
}

func routeOf(r *store.Route) v1gen.TeamRoute {
	muted := make([]v1gen.ReplyKind, 0, len(r.MutedReplies))
	for _, kind := range r.MutedReplies {
		muted = append(muted, v1gen.ReplyKind(kind))
	}
	return v1gen.TeamRoute{
		ID:           r.ID,
		TeamID:       r.TeamID,
		SourceID:     r.SourceID,
		SinkID:       r.SinkID,
		MutedReplies: muted,
		Etag:         etagOf(r.Version),
	}
}

func routeHeaders(r *store.Route) *v1gen.TeamRouteHeaders {
	return &v1gen.TeamRouteHeaders{
		ETag:     v1gen.NewOptString(etagOf(r.Version)),
		Response: routeOf(r),
	}
}
