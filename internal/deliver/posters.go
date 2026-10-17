package deliver

import (
	"context"
	"errors"
	"fmt"

	"github.com/patchcaster-app/patchcaster/internal/mattermost"
	"github.com/patchcaster-app/patchcaster/internal/platform"
	"github.com/patchcaster-app/patchcaster/internal/seal"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// ConnectionStore is what ConnectionPosters reads.
type ConnectionStore interface {
	Connection(ctx context.Context, id int64) (*store.Connection, error)
}

// ConnectionPosters posts with the stored credential of each chat
// connection. It reads the connection for every delivery, so a replaced
// credential applies to the next message (SYSREQ-034).
type ConnectionPosters struct {
	Store ConnectionStore
	Box   *seal.Box
}

// Poster returns a client of the chat connection with connectionID, or
// ErrSinkGone when the connection was removed.
func (c ConnectionPosters) Poster(ctx context.Context, connectionID int64) (Poster, error) {
	connection, err := c.Store.Connection(ctx, connectionID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrSinkGone
	}
	if err != nil {
		return nil, err
	}
	if platform.Kind(connection.Kind) != platform.Mattermost {
		return nil, fmt.Errorf("connection %d of kind %q delivers no messages", connectionID, connection.Kind)
	}
	credential, err := c.Box.Open(connection.Credential, store.ConnectionCredentialBinding(connection.ID))
	if err != nil {
		return nil, fmt.Errorf("open credential of connection %d: %w", connectionID, err)
	}
	return mattermost.New(connection.BaseURL, string(credential)), nil
}
