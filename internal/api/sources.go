package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/platform"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// ListSources pages through the sources the caller may see.
func (s *Service) ListSources(ctx context.Context, params v1gen.ListSourcesParams) (*v1gen.SourcePage, error) {
	afterID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, err
	}
	limit := params.Limit.Or(defaultPageSize)
	sources, err := s.store.Sources(ctx, visibleTo(ctx), afterID, limit+1)
	if err != nil {
		return nil, err
	}
	sources, next := pageOf(sources, limit, func(src store.Source) int64 { return src.ID })
	hookStates, err := s.sourceHooks(ctx, sources)
	if err != nil {
		return nil, err
	}
	items := make([]v1gen.Source, 0, len(sources))
	for i := range sources {
		items = append(items, sourceOf(&sources[i], hookStates[sources[i].ID]))
	}
	return &v1gen.SourcePage{Items: items, NextCursor: next}, nil
}

// CreateSource creates a source of a forge connection, for administrators.
func (s *Service) CreateSource(ctx context.Context, req *v1gen.SourceRequest) (*v1gen.Source, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	if _, err := s.connectionOfKind(ctx, req.ConnectionID, platform.Kind.IsForge, "a forge"); err != nil {
		return nil, err
	}
	kind, path, err := parseScope(req.Scope)
	if err != nil {
		return nil, err
	}
	created, err := s.store.CreateSource(ctx, actorOf(ctx), store.Source{
		ConnectionID: req.ConnectionID,
		Name:         req.Name,
		ScopeKind:    kind,
		ScopePath:    path,
	}, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	s.hooks.Poke(created.ConnectionID)
	hookStates, err := s.sourceHooks(ctx, []store.Source{*created})
	if err != nil {
		return nil, err
	}
	source := sourceOf(created, hookStates[created.ID])
	return &source, nil
}

// GetSource reads a source the caller may see.
func (s *Service) GetSource(ctx context.Context, params v1gen.GetSourceParams) (*v1gen.SourceHeaders, error) {
	source, err := s.store.Source(ctx, params.ID)
	if err != nil {
		return nil, storeProblem(err)
	}
	if err := s.mayReadOwned(ctx, source.Teams); err != nil {
		return nil, err
	}
	return s.sourceHeaders(ctx, source)
}

// UpdateSource changes a source's name and scope, for administrators.
func (s *Service) UpdateSource(ctx context.Context, req *v1gen.SourceUpdate, params v1gen.UpdateSourceParams) (*v1gen.SourceHeaders, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return nil, err
	}
	kind, path, err := parseScope(req.Scope)
	if err != nil {
		return nil, err
	}
	updated, err := s.store.UpdateSource(ctx, actorOf(ctx), store.Source{
		ID:        params.ID,
		Name:      req.Name,
		ScopeKind: kind,
		ScopePath: path,
		Version:   version,
	}, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	s.hooks.Poke(updated.ConnectionID)
	return s.sourceHeaders(ctx, updated)
}

// DeleteSource removes a source no route uses, for administrators.
func (s *Service) DeleteSource(ctx context.Context, params v1gen.DeleteSourceParams) error {
	if _, err := administrator(ctx); err != nil {
		return err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return err
	}
	// The connection names whose hooks follow the removal.
	source, err := s.store.Source(ctx, params.ID)
	if err != nil {
		return storeProblem(err)
	}
	if err := s.store.DeleteSource(ctx, actorOf(ctx), params.ID, version, s.now()); err != nil {
		return storeProblem(err)
	}
	s.hooks.Poke(source.ConnectionID)
	return nil
}

// ListSinks pages through the sinks the caller may see.
func (s *Service) ListSinks(ctx context.Context, params v1gen.ListSinksParams) (*v1gen.SinkPage, error) {
	afterID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, err
	}
	limit := params.Limit.Or(defaultPageSize)
	sinks, err := s.store.Sinks(ctx, visibleTo(ctx), afterID, limit+1)
	if err != nil {
		return nil, err
	}
	sinks, next := pageOf(sinks, limit, func(k store.Sink) int64 { return k.ID })
	health, err := s.sinkHealth(ctx, sinks)
	if err != nil {
		return nil, err
	}
	items := make([]v1gen.Sink, 0, len(sinks))
	for i := range sinks {
		items = append(items, sinkOf(&sinks[i], health[sinks[i].ID]))
	}
	return &v1gen.SinkPage{Items: items, NextCursor: next}, nil
}

// CreateSink creates a sink in a channel of a chat connection, for
// administrators, once the connection's bot is known to be able to post
// there (SYSREQ-045).
func (s *Service) CreateSink(ctx context.Context, req *v1gen.SinkRequest) (*v1gen.Sink, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	connection, err := s.connectionOfKind(ctx, req.ConnectionID, isChat, "a chat")
	if err != nil {
		return nil, err
	}
	channelName, err := s.checkChannel(ctx, connection, req.ChannelID)
	if err != nil {
		return nil, err
	}
	created, err := s.store.CreateSink(ctx, actorOf(ctx), store.Sink{
		ConnectionID: connection.ID,
		Name:         req.Name,
		ChannelID:    req.ChannelID,
		ChannelName:  channelName,
	}, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	health, err := s.sinkHealth(ctx, []store.Sink{*created})
	if err != nil {
		return nil, err
	}
	sink := sinkOf(created, health[created.ID])
	return &sink, nil
}

// GetSink reads a sink the caller may see.
func (s *Service) GetSink(ctx context.Context, params v1gen.GetSinkParams) (*v1gen.SinkHeaders, error) {
	sink, err := s.store.Sink(ctx, params.ID)
	if err != nil {
		return nil, storeProblem(err)
	}
	if err := s.mayReadOwned(ctx, sink.Teams); err != nil {
		return nil, err
	}
	return s.sinkHeaders(ctx, sink)
}

// UpdateSink changes a sink's name and channel, for administrators. A new
// channel is checked like a new sink.
func (s *Service) UpdateSink(ctx context.Context, req *v1gen.SinkUpdate, params v1gen.UpdateSinkParams) (*v1gen.SinkHeaders, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return nil, err
	}
	current, err := s.store.Sink(ctx, params.ID)
	if err != nil {
		return nil, storeProblem(err)
	}
	channelName := current.ChannelName
	if req.ChannelID != current.ChannelID {
		connection, err := s.store.Connection(ctx, current.ConnectionID)
		if err != nil {
			return nil, err
		}
		channelName, err = s.checkChannel(ctx, connection, req.ChannelID)
		if err != nil {
			return nil, err
		}
	}
	updated, err := s.store.UpdateSink(ctx, actorOf(ctx), store.Sink{
		ID:          current.ID,
		Name:        req.Name,
		ChannelID:   req.ChannelID,
		ChannelName: channelName,
		Version:     version,
	}, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	return s.sinkHeaders(ctx, updated)
}

// DeleteSink removes a sink no route uses, for administrators.
func (s *Service) DeleteSink(ctx context.Context, params v1gen.DeleteSinkParams) error {
	if _, err := administrator(ctx); err != nil {
		return err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return err
	}
	if err := s.store.DeleteSink(ctx, actorOf(ctx), params.ID, version, s.now()); err != nil {
		return storeProblem(err)
	}
	return nil
}

func isChat(kind platform.Kind) bool {
	return !kind.IsForge()
}

// connectionOfKind reads the connection a source or sink names, and refuses
// one of the wrong kind, naming the field for the form.
func (s *Service) connectionOfKind(ctx context.Context, id int64, ofKind func(platform.Kind) bool, kindName string) (*store.Connection, error) {
	connection, err := s.store.Connection(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, fieldProblem(http.StatusUnprocessableEntity, "the connection does not exist", "connection_id", "names no connection")
	}
	if err != nil {
		return nil, err
	}
	if !ofKind(platform.Kind(connection.Kind)) {
		return nil, fieldProblem(http.StatusUnprocessableEntity, "the connection is of the wrong kind",
			"connection_id", fmt.Sprintf("is no %s connection", kindName))
	}
	return connection, nil
}

// checkChannel asks the chat platform whether the connection's bot can post
// into the channel, and returns the channel's name. Each reason it cannot
// names the field that needs a change.
func (s *Service) checkChannel(ctx context.Context, connection *store.Connection, channelID string) (string, error) {
	baseURL, err := url.Parse(connection.BaseURL)
	if err != nil {
		return "", fmt.Errorf("read base URL of connection %d: %w", connection.ID, err)
	}
	credential, err := s.box.Open(connection.Credential, store.ConnectionCredentialBinding(connection.ID))
	if err != nil {
		return "", fmt.Errorf("open credential of connection %d: %w", connection.ID, err)
	}
	kind := platform.Kind(connection.Kind)
	name, err := s.platforms.CheckChannel(ctx, kind, baseURL, string(credential), channelID)
	var undeliverable *platform.UndeliverableError
	if errors.As(err, &undeliverable) {
		return "", fieldProblem(http.StatusUnprocessableEntity, undeliverable.Error(), "channel_id", undeliverable.Reason)
	}
	var refused *platform.RefusedError
	if errors.As(err, &refused) {
		return "", fieldProblem(http.StatusConflict, refused.Error(),
			"connection_id", fmt.Sprintf("%s refuses the connection's credential: %s", kind, refused.Answer))
	}
	var unreachable *platform.UnreachableError
	if errors.As(err, &unreachable) {
		s.log.WarnContext(ctx, "channel check failed", "connection", connection.ID, "err", err)
		return "", fieldProblem(http.StatusConflict, unreachable.Error(),
			"connection_id", fmt.Sprintf("no %s answers at the connection's address", kind))
	}
	return name, err
}

// parseScope reads a source's scope. A path loses surrounding slashes. The
// whole instance takes no path. A group needs one, and a repository needs
// its group as well as its name.
func parseScope(scope v1gen.SourceScope) (string, string, error) {
	path := strings.Trim(scope.Path.Or(""), "/")
	problem := func(message string) error {
		return fieldProblem(http.StatusUnprocessableEntity, "the scope names no "+string(scope.Kind), "scope.path", message)
	}
	switch scope.Kind {
	case v1gen.SourceScopeKindInstance:
		if path != "" {
			return "", "", problem("must be empty for the whole instance")
		}
	case v1gen.SourceScopeKindGroup:
		if path == "" {
			return "", "", problem("names the group, such as payments")
		}
	case v1gen.SourceScopeKindRepository:
		if !strings.Contains(path, "/") {
			return "", "", problem("names the repository with its group, such as payments/api")
		}
	}
	if strings.Contains(path, "//") {
		return "", "", problem("has an empty part")
	}
	return string(scope.Kind), path, nil
}

func sourceOf(src *store.Source, hook v1gen.SourceHook) v1gen.Source {
	return v1gen.Source{
		ID:           src.ID,
		ConnectionID: src.ConnectionID,
		Name:         src.Name,
		Scope:        scopeOf(store.HookScope{Kind: src.ScopeKind, Path: src.ScopePath}),
		Teams:        nonNilIDs(src.Teams),
		Hook:         hook,
		Etag:         etagOf(src.Version),
	}
}

// sourceHeaders renders one source with its hook state. The ETag covers
// the configuration alone, so the hook never fails a change.
func (s *Service) sourceHeaders(ctx context.Context, src *store.Source) (*v1gen.SourceHeaders, error) {
	hookStates, err := s.sourceHooks(ctx, []store.Source{*src})
	if err != nil {
		return nil, err
	}
	return &v1gen.SourceHeaders{
		ETag:     v1gen.NewOptString(etagOf(src.Version)),
		Response: sourceOf(src, hookStates[src.ID]),
	}, nil
}

func sinkOf(k *store.Sink, health v1gen.SinkHealth) v1gen.Sink {
	return v1gen.Sink{
		ID:           k.ID,
		ConnectionID: k.ConnectionID,
		Name:         k.Name,
		ChannelID:    k.ChannelID,
		ChannelName:  k.ChannelName,
		Teams:        nonNilIDs(k.Teams),
		Health:       health,
		Etag:         etagOf(k.Version),
	}
}

// sinkHeaders renders one sink with its health. The ETag covers the
// configuration alone, so health never fails a change.
func (s *Service) sinkHeaders(ctx context.Context, k *store.Sink) (*v1gen.SinkHeaders, error) {
	health, err := s.sinkHealth(ctx, []store.Sink{*k})
	if err != nil {
		return nil, err
	}
	return &v1gen.SinkHeaders{
		ETag:     v1gen.NewOptString(etagOf(k.Version)),
		Response: sinkOf(k, health[k.ID]),
	}, nil
}
