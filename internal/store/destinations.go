package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

// Destination is one route an event reaches: the sink it delivers to, the
// chat connection and channel of that sink, the replies the route mutes
// (SYSREQ-046, SYSREQ-047), and the scope of the route's source.
type Destination struct {
	RouteID      int64
	SinkID       int64
	ConnectionID int64
	ChannelID    string
	MutedReplies []string
	ScopeKind    string
	ScopePath    string
}

// Destinations lists the routes that an event of the forge connection with
// connectionID, in the repository with the full path repository, reaches.
// The list holds one entry per route, ordered by sink, so a sink that
// several routes reach appears once per route. Ingest reads it for every
// event, so a committed change applies to the next one (SYSREQ-034).
func (p *Postgres) Destinations(ctx context.Context, connectionID int64, repository string) ([]Destination, error) {
	rows, err := p.base.Destinations(ctx, sqlcgen.DestinationsParams{
		ConnectionID: connectionID,
		Repository:   repository,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve routes of %q: %w", repository, err)
	}
	destinations := make([]Destination, 0, len(rows))
	for _, row := range rows {
		d := Destination{
			RouteID:      row.RouteID,
			SinkID:       row.SinkID,
			ConnectionID: row.SinkConnectionID,
			ChannelID:    row.ChannelID,
			ScopeKind:    row.ScopeKind,
			ScopePath:    row.ScopePath,
		}
		if err := json.Unmarshal(row.MutedReplies, &d.MutedReplies); err != nil {
			return nil, fmt.Errorf("decode muted replies of a route to sink %d: %w", row.SinkID, err)
		}
		destinations = append(destinations, d)
	}
	return destinations, nil
}

// SigningTokensOf lists the sealed signing tokens of one connection, for
// verifying the webhooks it receives (SYSREQ-044).
func (p *Postgres) SigningTokensOf(ctx context.Context, connectionID int64) ([]SealedValue, error) {
	rows, err := p.base.SigningTokensOf(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("list signing tokens of connection %d: %w", connectionID, err)
	}
	values := make([]SealedValue, 0, len(rows))
	for _, row := range rows {
		values = append(values, SealedValue{ID: row.ID, Sealed: row.Token})
	}
	return values, nil
}
