package api

import (
	"context"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/auth"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// defaultTokenLifetimeDays is a token's lifetime when its owner names none.
// The description declares the same default and caps the lifetime at it
// (ADR-033).
const defaultTokenLifetimeDays = 90

// CreateToken creates an API token that acts with its owner's permissions.
// Only a login reaches this operation, so a token cannot mint its successor
// and outlive the cap on its lifetime (SYSREQ-040).
func (s *Service) CreateToken(ctx context.Context, req *v1gen.ApiTokenRequest) (*v1gen.CreatedApiToken, error) {
	owner := caller(ctx)
	secret, hash := auth.NewToken()
	now := s.now()
	lifetime := time.Duration(req.LifetimeDays.Or(defaultTokenLifetimeDays)) * 24 * time.Hour
	token, err := s.store.CreateToken(ctx, actorOf(ctx), store.NewAPIToken{
		PersonID:  owner.ID,
		Name:      req.Name,
		Hash:      hash,
		ExpiresAt: now.Add(lifetime),
	}, now)
	if err != nil {
		return nil, err
	}
	return &v1gen.CreatedApiToken{
		Token:  tokenOf(token),
		Secret: secret,
	}, nil
}

// ListTokens pages through the caller's own tokens.
func (s *Service) ListTokens(ctx context.Context, params v1gen.ListTokensParams) (*v1gen.ApiTokenPage, error) {
	afterID, err := decodeCursor(params.Cursor)
	if err != nil {
		return nil, err
	}
	limit := params.Limit.Or(defaultPageSize)
	tokens, err := s.store.Tokens(ctx, caller(ctx).ID, afterID, limit+1)
	if err != nil {
		return nil, err
	}
	tokens, next := pageOf(tokens, limit, func(t store.APIToken) int64 { return t.ID })
	items := make([]v1gen.ApiToken, 0, len(tokens))
	for i := range tokens {
		items = append(items, tokenOf(&tokens[i]))
	}
	return &v1gen.ApiTokenPage{Items: items, NextCursor: next}, nil
}

// RevokeToken revokes one of the caller's own tokens. Somebody else's token
// answers 404, as an unknown one does.
func (s *Service) RevokeToken(ctx context.Context, params v1gen.RevokeTokenParams) error {
	err := s.store.RevokeToken(ctx, actorOf(ctx), caller(ctx).ID, params.ID, s.now())
	if err != nil {
		return storeProblem(err)
	}
	return nil
}

// RevokeAccess revokes all tokens and ends all sessions of a person, for
// administrators (SYSREQ-040).
func (s *Service) RevokeAccess(ctx context.Context, params v1gen.RevokeAccessParams) (*v1gen.AccessRevoked, error) {
	if _, err := administrator(ctx); err != nil {
		return nil, err
	}
	revoked, err := s.store.RevokeAccess(ctx, actorOf(ctx), params.ID, s.now())
	if err != nil {
		return nil, storeProblem(err)
	}
	return &v1gen.AccessRevoked{RevokedTokens: revoked}, nil
}

func tokenOf(t *store.APIToken) v1gen.ApiToken {
	token := v1gen.ApiToken{
		ID:        t.ID,
		Name:      t.Name,
		CreatedAt: t.CreatedAt,
		ExpiresAt: t.ExpiresAt,
	}
	if !t.RevokedAt.IsZero() {
		token.RevokedAt = v1gen.NewOptDateTime(t.RevokedAt)
	}
	return token
}
