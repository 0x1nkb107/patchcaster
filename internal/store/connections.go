package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

// ErrNameTaken reports a name that another item of the same kind already
// has.
var ErrNameTaken = errors.New("the name is taken")

// Connection is what PatchCaster acts with on one forge or chat platform
// (SCEN-014). Kind names the platform. Credential is sealed with the
// instance key for ConnectionCredentialBinding, and nil for a forge
// connection without API access. Account is whom the credential acted as
// when it was last checked.
type Connection struct {
	ID         int64
	Kind       string
	Name       string
	BaseURL    string
	Credential []byte
	Account    string
	Version    int64
}

// SigningToken is a webhook signing token of a forge connection, without
// its value (SYSREQ-044).
type SigningToken struct {
	ID           int64
	ConnectionID int64
	Label        string
	CreatedAt    time.Time
}

// NewSigningToken is a signing token about to be added under an id from
// ReserveSigningTokenID. Sealed is the token sealed for SigningTokenBinding
// of that id.
type NewSigningToken struct {
	ID           int64
	ConnectionID int64
	Label        string
	Sealed       []byte
}

// SealedValue is one stored secret, for a reseal.
type SealedValue struct {
	ID     int64
	Sealed []byte
}

// ConnectionCredentialBinding is what a connection's credential is sealed
// for. Each credential opens only in its own row, so a sealed value copied
// into another connection opens nothing.
func ConnectionCredentialBinding(id int64) string {
	return "connections.credential." + strconv.FormatInt(id, 10)
}

// SigningTokenBinding is what a signing token is sealed for, the same way.
func SigningTokenBinding(id int64) string {
	return "signing_tokens.token." + strconv.FormatInt(id, 10)
}

// connectionFrom reads a connection row. Every connection query returns the
// same columns, so their row types convert to this one.
func connectionFrom(row sqlcgen.ConnectionGetRow) *Connection {
	return &Connection{
		ID:         row.ID,
		Kind:       row.Kind,
		Name:       row.Name,
		BaseURL:    row.BaseUrl,
		Credential: row.Credential,
		Account:    row.Account,
		Version:    row.Version,
	}
}

// connectionTarget is how the audit log shows a connection. It leaves the
// credential out. A new one shows only as "secret changed".
func connectionTarget(c *Connection, newCredential bool) map[string]any {
	target := map[string]any{
		"kind":     c.Kind,
		"name":     c.Name,
		"base_url": c.BaseURL,
		"account":  c.Account,
	}
	if newCredential {
		target["credential"] = secretChanged
	}
	return target
}

// connectionTargetID renders a connection's id as an audit target.
func connectionTargetID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// nameTaken turns a unique violation into ErrNameTaken.
func nameTaken(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrNameTaken
	}
	return err
}

// ReserveConnectionID reserves the id of a connection about to be created,
// so that its credential can be sealed for it first.
func (p *Postgres) ReserveConnectionID(ctx context.Context) (int64, error) {
	id, err := p.base.ConnectionNextID(ctx)
	if err != nil {
		return 0, fmt.Errorf("reserve connection id: %w", err)
	}
	return id, nil
}

// CreateConnection stores a new connection under a reserved id, and
// records it in the audit log.
func (p *Postgres) CreateConnection(ctx context.Context, actor Actor, c Connection, now time.Time) (*Connection, error) {
	var created *Connection
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		row, err := q.ConnectionCreate(ctx, sqlcgen.ConnectionCreateParams{
			ID:         c.ID,
			Kind:       c.Kind,
			Name:       c.Name,
			BaseUrl:    c.BaseURL,
			Credential: c.Credential,
			Account:    c.Account,
			CreatedAt:  now,
		})
		if err != nil {
			return nameTaken(fmt.Errorf("store connection: %w", err))
		}
		created = connectionFrom(sqlcgen.ConnectionGetRow(row))
		return audit(ctx, q, actor, now, change{
			action:     "connection.create",
			targetKind: "connection",
			targetID:   connectionTargetID(created.ID),
			after:      connectionTarget(created, c.Credential != nil),
		})
	})
	return created, err
}

// readConnection reads a connection in the transaction q belongs to.
func readConnection(ctx context.Context, q *sqlcgen.Queries, id int64) (*Connection, error) {
	row, err := q.ConnectionGet(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read connection %d: %w", id, err)
	}
	return connectionFrom(row), nil
}

// Connection reads one connection, or returns ErrNotFound.
func (p *Postgres) Connection(ctx context.Context, id int64) (*Connection, error) {
	return readConnection(ctx, p.base, id)
}

// Connections pages through connections in the order they were created,
// starting after the connection with afterID.
func (p *Postgres) Connections(ctx context.Context, afterID int64, limit int) ([]Connection, error) {
	rows, err := p.base.ConnectionList(ctx, sqlcgen.ConnectionListParams{
		AfterID:  afterID,
		PageSize: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	connections := make([]Connection, 0, len(rows))
	for _, row := range rows {
		connections = append(connections, *connectionFrom(sqlcgen.ConnectionGetRow(row)))
	}
	return connections, nil
}

// UpdateConnection changes a connection's name, base URL, credential and
// account at the version in c, the one the caller read. A nil Credential
// keeps the stored one. The kind never changes.
func (p *Postgres) UpdateConnection(ctx context.Context, actor Actor, c Connection, now time.Time) (*Connection, error) {
	var updated *Connection
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		before, err := readConnection(ctx, q, c.ID)
		if err != nil {
			return err
		}
		if before.Version != c.Version {
			return ErrVersionMismatch
		}
		n, err := q.ConnectionUpdate(ctx, sqlcgen.ConnectionUpdateParams{
			ID:         c.ID,
			Name:       c.Name,
			BaseUrl:    c.BaseURL,
			Credential: c.Credential,
			Account:    c.Account,
			UpdatedAt:  now,
			Version:    c.Version,
		})
		if err != nil {
			return nameTaken(fmt.Errorf("update connection %d: %w", c.ID, err))
		}
		if n == 0 {
			return ErrVersionMismatch
		}
		updated, err = readConnection(ctx, q, c.ID)
		if err != nil {
			return err
		}
		return audit(ctx, q, actor, now, change{
			action:     "connection.update",
			targetKind: "connection",
			targetID:   connectionTargetID(c.ID),
			before:     connectionTarget(before, false),
			after:      connectionTarget(updated, c.Credential != nil),
		})
	})
	return updated, err
}

// DeleteConnection removes a connection at the version the caller read,
// with its signing tokens. A connection that sources or sinks use cannot
// go, and neither can one whose registered hooks remain.
func (p *Postgres) DeleteConnection(ctx context.Context, actor Actor, id, version int64, now time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		before, err := readConnection(ctx, q, id)
		if err != nil {
			return err
		}
		n, err := q.ConnectionDelete(ctx, sqlcgen.ConnectionDeleteParams{ID: id, Version: version})
		if err != nil {
			return referenceProblem(fmt.Errorf("delete connection %d: %w", id, err), nil)
		}
		if n == 0 {
			return ErrVersionMismatch
		}
		return audit(ctx, q, actor, now, change{
			action:     "connection.delete",
			targetKind: "connection",
			targetID:   connectionTargetID(id),
			before:     connectionTarget(before, false),
		})
	})
}

// ReserveSigningTokenID reserves the id of a signing token about to be
// added, so that it can be sealed for it first.
func (p *Postgres) ReserveSigningTokenID(ctx context.Context) (int64, error) {
	id, err := p.base.SigningTokenNextID(ctx)
	if err != nil {
		return 0, fmt.Errorf("reserve signing token id: %w", err)
	}
	return id, nil
}

// AddSigningToken stores a sealed signing token for a connection, and
// records it in the audit log without its value.
func (p *Postgres) AddSigningToken(ctx context.Context, actor Actor, t NewSigningToken, now time.Time) (*SigningToken, error) {
	err := p.inTx(ctx, func(q *sqlcgen.Queries) error {
		if _, err := readConnection(ctx, q, t.ConnectionID); err != nil {
			return err
		}
		err := q.SigningTokenAdd(ctx, sqlcgen.SigningTokenAddParams{
			ID:           t.ID,
			ConnectionID: t.ConnectionID,
			Label:        t.Label,
			Token:        t.Sealed,
			CreatedAt:    now,
		})
		if err != nil {
			return fmt.Errorf("store signing token: %w", err)
		}
		return audit(ctx, q, actor, now, change{
			action:     "signing_token.add",
			targetKind: "connection",
			targetID:   connectionTargetID(t.ConnectionID),
			after:      map[string]any{"signing_token": t.ID, "label": t.Label},
		})
	})
	if err != nil {
		return nil, err
	}
	return &SigningToken{
		ID:           t.ID,
		ConnectionID: t.ConnectionID,
		Label:        t.Label,
		CreatedAt:    now,
	}, nil
}

// SigningTokens pages through a connection's signing tokens, without their
// values, in the order they were added.
func (p *Postgres) SigningTokens(ctx context.Context, connectionID, afterID int64, limit int) ([]SigningToken, error) {
	rows, err := p.base.SigningTokenList(ctx, sqlcgen.SigningTokenListParams{
		ConnectionID: connectionID,
		AfterID:      afterID,
		PageSize:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list signing tokens: %w", err)
	}
	tokens := make([]SigningToken, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, SigningToken{
			ID:           row.ID,
			ConnectionID: row.ConnectionID,
			Label:        row.Label,
			CreatedAt:    row.CreatedAt,
		})
	}
	return tokens, nil
}

// RemoveSigningToken removes one signing token of a connection, or returns
// ErrNotFound.
func (p *Postgres) RemoveSigningToken(ctx context.Context, actor Actor, connectionID, tokenID int64, now time.Time) error {
	return p.inTx(ctx, func(q *sqlcgen.Queries) error {
		row, err := q.SigningTokenGet(ctx, sqlcgen.SigningTokenGetParams{ID: tokenID, ConnectionID: connectionID})
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read signing token %d: %w", tokenID, err)
		}
		// The token of a hook PatchCaster registered stays until the hook
		// is gone, so that the hook's deliveries keep verifying.
		if _, err := q.SigningTokenRemove(ctx, sqlcgen.SigningTokenRemoveParams{ID: tokenID, ConnectionID: connectionID}); err != nil {
			return referenceProblem(fmt.Errorf("remove signing token %d: %w", tokenID, err), nil)
		}
		return audit(ctx, q, actor, now, change{
			action:     "signing_token.remove",
			targetKind: "connection",
			targetID:   connectionTargetID(connectionID),
			before:     map[string]any{"signing_token": row.ID, "label": row.Label},
		})
	})
}

// ConnectionCredentials lists every sealed connection credential, for a
// reseal.
func (p *Postgres) ConnectionCredentials(ctx context.Context) ([]SealedValue, error) {
	rows, err := p.base.ConnectionCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("list connection credentials: %w", err)
	}
	values := make([]SealedValue, 0, len(rows))
	for _, row := range rows {
		values = append(values, SealedValue{ID: row.ID, Sealed: row.Credential})
	}
	return values, nil
}

// ReplaceConnectionCredential stores a credential sealed again, as long as
// the stored one is still sealed. It reports false when the credential
// changed meanwhile. A reseal changes no setting, so it is not audited.
func (p *Postgres) ReplaceConnectionCredential(ctx context.Context, id int64, sealed, resealed []byte) (bool, error) {
	n, err := p.base.ConnectionCredentialReplace(ctx, sqlcgen.ConnectionCredentialReplaceParams{
		ID:       id,
		Sealed:   sealed,
		Resealed: resealed,
	})
	if err != nil {
		return false, fmt.Errorf("replace credential of connection %d: %w", id, err)
	}
	return n == 1, nil
}

// SealedSigningTokens lists every sealed signing token, for a reseal.
func (p *Postgres) SealedSigningTokens(ctx context.Context) ([]SealedValue, error) {
	rows, err := p.base.SigningTokensSealed(ctx)
	if err != nil {
		return nil, fmt.Errorf("list signing tokens: %w", err)
	}
	values := make([]SealedValue, 0, len(rows))
	for _, row := range rows {
		values = append(values, SealedValue{ID: row.ID, Sealed: row.Token})
	}
	return values, nil
}

// ReplaceSigningToken stores a signing token sealed again, as long as the
// stored one is still sealed.
func (p *Postgres) ReplaceSigningToken(ctx context.Context, id int64, sealed, resealed []byte) (bool, error) {
	n, err := p.base.SigningTokenReplace(ctx, sqlcgen.SigningTokenReplaceParams{
		ID:       id,
		Sealed:   sealed,
		Resealed: resealed,
	})
	if err != nil {
		return false, fmt.Errorf("replace signing token %d: %w", id, err)
	}
	return n == 1, nil
}
