package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/ingest"
	"github.com/patchcaster-app/patchcaster/internal/platform"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// ListConnections pages through the connections, for administrators.
func (s *Service) ListConnections(ctx context.Context, params v1gen.ListConnectionsParams) (*v1gen.ConnectionPage, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	afterID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, err
	}
	limit := params.Limit.Or(defaultPageSize)
	connections, err := s.store.Connections(ctx, afterID, limit+1)
	if err != nil {
		return nil, err
	}
	connections, next := pageOf(connections, limit, func(c store.Connection) int64 { return c.ID })
	health, err := s.connectionHealth(ctx, connections)
	if err != nil {
		return nil, err
	}
	items := make([]v1gen.Connection, 0, len(connections))
	for i := range connections {
		items = append(items, s.connectionOf(&connections[i], health[connections[i].ID]))
	}
	return &v1gen.ConnectionPage{Items: items, NextCursor: next}, nil
}

// CreateConnection connects a forge or chat platform, for administrators. A
// credential is checked against the platform and sealed before it is
// stored (SCEN-014).
func (s *Service) CreateConnection(ctx context.Context, req *v1gen.ConnectionRequest) (*v1gen.Connection, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	kind := platform.Kind(req.Kind)
	baseURL, err := parseBaseURL(req.BaseURL)
	if err != nil {
		return nil, err
	}
	credential, hasCredential := req.Credential.Get()
	if !hasCredential && kind.NeedsCredential() {
		return nil, fieldProblem(http.StatusUnprocessableEntity, "a chat connection posts with its bot's token",
			"credential", fmt.Sprintf("is required for a %s connection", kind))
	}
	account, err := s.checkCredential(ctx, kind, baseURL, credential, hasCredential)
	if err != nil {
		return nil, err
	}
	id, err := s.store.ReserveConnectionID(ctx)
	if err != nil {
		return nil, err
	}
	sealed, err := s.sealCredential(id, credential, hasCredential)
	if err != nil {
		return nil, err
	}
	created, err := s.store.CreateConnection(ctx, actorOf(ctx), store.Connection{
		ID:         id,
		Kind:       string(kind),
		Name:       req.Name,
		BaseURL:    baseURL.String(),
		Credential: sealed,
		Account:    account,
	}, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	health, err := s.connectionHealth(ctx, []store.Connection{*created})
	if err != nil {
		return nil, err
	}
	connection := s.connectionOf(created, health[created.ID])
	return &connection, nil
}

// GetConnection reads one connection, without its credential, for
// administrators.
func (s *Service) GetConnection(ctx context.Context, params v1gen.GetConnectionParams) (*v1gen.ConnectionHeaders, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	connection, err := s.store.Connection(ctx, params.ID)
	if err != nil {
		return nil, storeProblem(err)
	}
	return s.connectionHeaders(ctx, connection)
}

// UpdateConnection changes a connection's name, base URL or credential, for
// administrators. Whatever the platform would see differently is checked
// against it first.
func (s *Service) UpdateConnection(ctx context.Context, req *v1gen.ConnectionUpdate, params v1gen.UpdateConnectionParams) (*v1gen.ConnectionHeaders, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return nil, err
	}
	current, err := s.store.Connection(ctx, params.ID)
	if err != nil {
		return nil, storeProblem(err)
	}
	baseURL, err := parseBaseURL(req.BaseURL)
	if err != nil {
		return nil, err
	}
	account, err := s.accountAfterUpdate(ctx, current, baseURL, req.Credential)
	if err != nil {
		return nil, err
	}
	credential, newCredential := req.Credential.Get()
	sealed, err := s.sealCredential(current.ID, credential, newCredential)
	if err != nil {
		return nil, err
	}
	updated, err := s.store.UpdateConnection(ctx, actorOf(ctx), store.Connection{
		ID:         current.ID,
		Name:       req.Name,
		BaseURL:    baseURL.String(),
		Credential: sealed,
		Account:    account,
		Version:    version,
	}, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	// A new or removed credential, or a new base URL, changes which hooks
	// PatchCaster can register (SYSREQ-044).
	if platform.Kind(updated.Kind).IsForge() {
		s.hooks.Poke(updated.ID)
	}
	return s.connectionHeaders(ctx, updated)
}

// DeleteConnection removes a connection and its signing tokens, for
// administrators.
func (s *Service) DeleteConnection(ctx context.Context, params v1gen.DeleteConnectionParams) error {
	if _, err := administrator(ctx); err != nil {
		return err
	}
	version, err := versionOf(params.IfMatch)
	if err != nil {
		return err
	}
	err = s.store.DeleteConnection(ctx, actorOf(ctx), params.ID, version, s.now())
	if err != nil {
		return storeProblem(err)
	}
	return nil
}

// ListSigningTokens pages through a connection's webhook signing tokens,
// without their values, for administrators.
func (s *Service) ListSigningTokens(ctx context.Context, params v1gen.ListSigningTokensParams) (*v1gen.SigningTokenPage, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	if _, err := s.store.Connection(ctx, params.ID); err != nil {
		return nil, storeProblem(err)
	}
	afterID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, err
	}
	limit := params.Limit.Or(defaultPageSize)
	tokens, err := s.store.SigningTokens(ctx, params.ID, afterID, limit+1)
	if err != nil {
		return nil, err
	}
	tokens, next := pageOf(tokens, limit, func(t store.SigningToken) int64 { return t.ID })
	items := make([]v1gen.SigningToken, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, signingTokenOf(token))
	}
	return &v1gen.SigningTokenPage{Items: items, NextCursor: next}, nil
}

// AddSigningToken adds a webhook signing token to a forge connection, for
// administrators. The token is checked for its form and sealed before it is
// stored.
func (s *Service) AddSigningToken(ctx context.Context, req *v1gen.SigningTokenRequest, params v1gen.AddSigningTokenParams) (*v1gen.SigningToken, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	connection, err := s.store.Connection(ctx, params.ID)
	if err != nil {
		return nil, storeProblem(err)
	}
	if !platform.Kind(connection.Kind).IsForge() {
		return nil, newProblem(http.StatusConflict, "only forge connections receive webhooks, so only they hold signing tokens")
	}
	if _, err := ingest.ParseSigningToken(req.Token); err != nil {
		return nil, fieldProblem(http.StatusUnprocessableEntity, "the signing token is not in the form the forge shows", "token", err.Error())
	}
	id, err := s.store.ReserveSigningTokenID(ctx)
	if err != nil {
		return nil, err
	}
	sealed, err := s.box.Seal([]byte(req.Token), store.SigningTokenBinding(id))
	if err != nil {
		return nil, fmt.Errorf("seal signing token: %w", err)
	}
	added, err := s.store.AddSigningToken(ctx, actorOf(ctx), store.NewSigningToken{
		ID:           id,
		ConnectionID: connection.ID,
		Label:        req.Label,
		Sealed:       sealed,
	}, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	token := signingTokenOf(*added)
	return &token, nil
}

// RemoveSigningToken removes a webhook signing token, for administrators.
func (s *Service) RemoveSigningToken(ctx context.Context, params v1gen.RemoveSigningTokenParams) error {
	if _, err := administrator(ctx); err != nil {
		return err
	}
	err := s.store.RemoveSigningToken(ctx, actorOf(ctx), params.ID, params.TokenID, s.now())
	if err != nil {
		return storeProblem(err)
	}
	return nil
}

// parseBaseURL reads a platform's base URL, naming the field for the form.
func parseBaseURL(raw string) (*url.URL, error) {
	baseURL, err := platform.ParseBaseURL(raw)
	if err != nil {
		return nil, fieldProblem(http.StatusUnprocessableEntity, "the base URL is no platform address", "base_url", err.Error())
	}
	return baseURL, nil
}

// accountAfterUpdate checks whatever the platform would see differently
// after an update: a new credential, or the stored one at a new base URL. It
// returns whom the credential then acts as.
func (s *Service) accountAfterUpdate(ctx context.Context, current *store.Connection, baseURL *url.URL, given v1gen.OptString) (string, error) {
	kind := platform.Kind(current.Kind)
	if credential, ok := given.Get(); ok {
		return s.checkCredential(ctx, kind, baseURL, credential, true)
	}
	unchanged := current.Credential == nil || baseURL.String() == current.BaseURL
	if unchanged {
		return current.Account, nil
	}
	stored, err := s.box.Open(current.Credential, store.ConnectionCredentialBinding(current.ID))
	if err != nil {
		return "", fmt.Errorf("open credential of connection %d: %w", current.ID, err)
	}
	return s.checkCredential(ctx, kind, baseURL, string(stored), true)
}

// checkCredential asks the platform whom a credential acts as. Without a
// credential there is nothing to ask, and no account.
func (s *Service) checkCredential(ctx context.Context, kind platform.Kind, baseURL *url.URL, credential string, hasCredential bool) (string, error) {
	if !hasCredential {
		return "", nil
	}
	account, err := s.platforms.Check(ctx, kind, baseURL, credential)
	var refused *platform.RefusedError
	if errors.As(err, &refused) {
		return "", fieldProblem(http.StatusUnprocessableEntity, refused.Error(),
			"credential", fmt.Sprintf("%s refused it: %s", kind.Name(), refused.Answer))
	}
	var unreachable *platform.UnreachableError
	if errors.As(err, &unreachable) {
		s.log.WarnContext(ctx, "connection check failed", "kind", kind, "base_url", baseURL.String(), "err", err)
		return "", fieldProblem(http.StatusUnprocessableEntity, unreachable.Error(),
			"base_url", fmt.Sprintf("no %s answers at this address", kind.Name()))
	}
	return account, err
}

// sealCredential seals a new credential for the connection with id. Without
// a new one there is nothing to seal, and the store keeps what it has.
func (s *Service) sealCredential(id int64, credential string, isNew bool) ([]byte, error) {
	if !isNew {
		return nil, nil
	}
	sealed, err := s.box.Seal([]byte(credential), store.ConnectionCredentialBinding(id))
	if err != nil {
		return nil, fmt.Errorf("seal credential: %w", err)
	}
	return sealed, nil
}

// webhookURL is where a forge sends a connection's webhooks: below the
// webhook listener's public address, by the connection's id.
func (s *Service) webhookURL(id int64) string {
	return ingest.EndpointURL(s.publicURL, id)
}

func (s *Service) connectionOf(c *store.Connection, health v1gen.ConnectionHealth) v1gen.Connection {
	connection := v1gen.Connection{
		ID:            c.ID,
		Kind:          v1gen.ConnectionKind(c.Kind),
		Name:          c.Name,
		BaseURL:       c.BaseURL,
		Account:       c.Account,
		HasCredential: c.Credential != nil,
		Health:        health,
		Etag:          etagOf(c.Version),
	}
	if platform.Kind(c.Kind).IsForge() {
		connection.WebhookURL = v1gen.NewOptString(s.webhookURL(c.ID))
	}
	return connection
}

// connectionHeaders renders one connection with its health. The ETag
// covers the configuration alone, so health never fails a change.
func (s *Service) connectionHeaders(ctx context.Context, c *store.Connection) (*v1gen.ConnectionHeaders, error) {
	health, err := s.connectionHealth(ctx, []store.Connection{*c})
	if err != nil {
		return nil, err
	}
	return &v1gen.ConnectionHeaders{
		ETag:     v1gen.NewOptString(etagOf(c.Version)),
		Response: s.connectionOf(c, health[c.ID]),
	}, nil
}

func signingTokenOf(t store.SigningToken) v1gen.SigningToken {
	return v1gen.SigningToken{
		ID:        t.ID,
		Label:     t.Label,
		CreatedAt: t.CreatedAt,
	}
}
