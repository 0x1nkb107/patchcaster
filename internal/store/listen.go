package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// outboxChannel is the notification channel that Accept signals for every
// enqueued row and that OutboxListener subscribes to (ADR-019).
const outboxChannel = "patchcaster_outbox"

// listenerProbeTimeout bounds the wait for a session's own probe
// notification. A direct connection answers in milliseconds, so running out
// means notifications do not reach this connection at all.
const listenerProbeTimeout = 10 * time.Second

// listenerCloseTimeout bounds the goodbye to the server when a session ends,
// so a dead peer cannot hold up the reconnect.
const listenerCloseTimeout = 5 * time.Second

// OutboxListener reports committed outbox rows through PostgreSQL
// LISTEN/NOTIFY (ADR-019). Each session holds its own connection outside the
// pool, because a listening connection stays checked out for as long as it
// listens.
type OutboxListener struct {
	config *pgx.ConnConfig
}

// NewOutboxListener prepares sessions against the database the pool uses.
func NewOutboxListener(databaseURL string) (*OutboxListener, error) {
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	return &OutboxListener{config: config}, nil
}

// Listen holds one listening session until its connection fails or ctx ends,
// and returns the cause. It calls wake once as soon as the session listens,
// because rows committed before then notified nobody, and once for every
// notification after that. A wake means "claim again", never "claim this
// row": notifications carry no payload.
func (l *OutboxListener) Listen(ctx context.Context, wake func()) error {
	conn, err := pgx.ConnectConfig(ctx, l.config)
	if err != nil {
		return fmt.Errorf("connect outbox listener: %w", err)
	}
	defer closeListenerConn(conn)

	if err := subscribe(ctx, conn); err != nil {
		return err
	}
	for {
		wake()
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return fmt.Errorf("wait for outbox notification: %w", err)
		}
	}
}

// subscribe starts listening and proves that notifications reach this
// connection by sending one to itself. A transaction-mode pooler between
// PatchCaster and PostgreSQL accepts LISTEN but never forwards
// notifications, and only this round trip exposes that.
func subscribe(ctx context.Context, conn *pgx.Conn) error {
	channel := pgx.Identifier{outboxChannel}.Sanitize()
	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		return fmt.Errorf("listen on outbox channel: %w", err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_notify($1, '')", outboxChannel); err != nil {
		return fmt.Errorf("send listener probe: %w", err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, listenerProbeTimeout)
	defer cancel()
	_, err := conn.WaitForNotification(probeCtx)
	if err == nil {
		return nil
	}

	probeExpired := ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded)
	if probeExpired {
		return fmt.Errorf("await listener probe: none within %s; notifications need a "+
			"direct or session-pooled connection", listenerProbeTimeout)
	}
	return fmt.Errorf("await listener probe: %w", err)
}

// closeListenerConn ends a session's connection with a fresh, bounded
// context: the session's own context has usually ended already.
func closeListenerConn(conn *pgx.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), listenerCloseTimeout)
	defer cancel()
	_ = conn.Close(ctx)
}
