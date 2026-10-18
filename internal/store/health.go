package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/store/sqlcgen"
)

// maxHealthError bounds the error text a sink's health keeps, in runes. A
// platform's answer can be long, and the first part says what went wrong.
const maxHealthError = 1000

// SinkHealth is how messages reach a sink (SYSREQ-064). A zero time means
// never: no waiting message, no success or no failure yet.
type SinkHealth struct {
	Waiting       int64
	OldestWaiting time.Time
	LastSuccess   time.Time
	LastFailure   time.Time
	LastError     string
}

// ForgeHealth is how the deliveries of a forge connection arrive
// (SYSREQ-064). A zero time means never.
type ForgeHealth struct {
	LastDelivery   time.Time
	LastRejection  time.Time
	RejectionCause string
}

// SinkQueue is the queue of one sink, for the waiting gauges.
type SinkQueue struct {
	SinkID        int64
	ConnectionID  int64
	Waiting       int64
	OldestWaiting time.Time
}

// timeOrZero reads a nullable timestamp that sqlc cannot type, such as an
// aggregate over a left join.
func timeOrZero(v any) (time.Time, error) {
	switch t := v.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return t, nil
	default:
		return time.Time{}, fmt.Errorf("read a timestamp from %T", v)
	}
}

// truncated keeps the first maxRunes runes of s.
func truncated(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

// idsJSON encodes ids for the jsonb array parameters of the queries.
func idsJSON(ids []int64) (json.RawMessage, error) {
	if ids == nil {
		ids = []int64{}
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encode ids: %w", err)
	}
	return raw, nil
}

func sinkHealthFrom(lastSuccess, lastFailure sql.NullTime, lastError sql.NullString, waiting int64, oldest any) (SinkHealth, error) {
	oldestWaiting, err := timeOrZero(oldest)
	if err != nil {
		return SinkHealth{}, err
	}
	return SinkHealth{
		Waiting:       waiting,
		OldestWaiting: oldestWaiting,
		LastSuccess:   lastSuccess.Time,
		LastFailure:   lastFailure.Time,
		LastError:     lastError.String,
	}, nil
}

// summed sums up the health of a chat connection's sinks: all their
// waiting messages, the oldest of them, the latest success, and the latest
// failure with its error.
func summed(sinks []SinkHealth) SinkHealth {
	var sum SinkHealth
	for _, h := range sinks {
		sum.Waiting += h.Waiting
		if !h.OldestWaiting.IsZero() && (sum.OldestWaiting.IsZero() || h.OldestWaiting.Before(sum.OldestWaiting)) {
			sum.OldestWaiting = h.OldestWaiting
		}
		if h.LastSuccess.After(sum.LastSuccess) {
			sum.LastSuccess = h.LastSuccess
		}
		if h.LastFailure.After(sum.LastFailure) {
			sum.LastFailure = h.LastFailure
			sum.LastError = h.LastError
		}
	}
	return sum
}

// RecordSinkSuccess records that the platform accepted a message for the
// sink at at.
func (p *Postgres) RecordSinkSuccess(ctx context.Context, sinkID int64, at time.Time) error {
	err := p.base.SinkHealthSucceeded(ctx, sqlcgen.SinkHealthSucceededParams{
		SinkID: sinkID,
		At:     sql.NullTime{Time: at, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("record success of sink %d: %w", sinkID, err)
	}
	return nil
}

// RecordSinkFailure records that an attempt at the sink's platform failed
// at at, and why. The error must name no secret.
func (p *Postgres) RecordSinkFailure(ctx context.Context, sinkID int64, at time.Time, reason string) error {
	err := p.base.SinkHealthFailed(ctx, sqlcgen.SinkHealthFailedParams{
		SinkID: sinkID,
		At:     sql.NullTime{Time: at, Valid: true},
		Error:  sql.NullString{String: truncated(reason, maxHealthError), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("record failure of sink %d: %w", sinkID, err)
	}
	return nil
}

// RecordForgeDelivery records that an authentic delivery of the forge
// connection arrived at at.
func (p *Postgres) RecordForgeDelivery(ctx context.Context, connectionID int64, at time.Time) error {
	err := p.base.ConnectionHealthDelivered(ctx, sqlcgen.ConnectionHealthDeliveredParams{
		ConnectionID: connectionID,
		At:           sql.NullTime{Time: at, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("record delivery of connection %d: %w", connectionID, err)
	}
	return nil
}

// RecordForgeRejection records that a delivery of the forge connection was
// rejected at at, and its cause.
func (p *Postgres) RecordForgeRejection(ctx context.Context, connectionID int64, at time.Time, cause string) error {
	err := p.base.ConnectionHealthRejected(ctx, sqlcgen.ConnectionHealthRejectedParams{
		ConnectionID: connectionID,
		At:           sql.NullTime{Time: at, Valid: true},
		Cause:        sql.NullString{String: cause, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("record rejection of connection %d: %w", connectionID, err)
	}
	return nil
}

// SinkHealth reads the health of the sinks with ids, by sink id. A sink
// that does not exist is missing from the result.
func (p *Postgres) SinkHealth(ctx context.Context, ids []int64) (map[int64]SinkHealth, error) {
	raw, err := idsJSON(ids)
	if err != nil {
		return nil, err
	}
	rows, err := p.base.SinkHealthList(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("read sink health: %w", err)
	}
	health := make(map[int64]SinkHealth, len(rows))
	for _, row := range rows {
		h, err := sinkHealthFrom(row.LastSuccessAt, row.LastFailureAt, row.LastError, row.Waiting, row.OldestWaiting)
		if err != nil {
			return nil, fmt.Errorf("read health of sink %d: %w", row.ID, err)
		}
		health[row.ID] = h
	}
	return health, nil
}

// ChatConnectionHealth reads the health of the chat connections with ids,
// by connection id: the health of their sinks, summed up. A connection
// without sinks has zero health.
func (p *Postgres) ChatConnectionHealth(ctx context.Context, ids []int64) (map[int64]SinkHealth, error) {
	raw, err := idsJSON(ids)
	if err != nil {
		return nil, err
	}
	rows, err := p.base.SinkHealthOfConnections(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("read chat connection health: %w", err)
	}
	sinksOf := make(map[int64][]SinkHealth, len(ids))
	for _, row := range rows {
		h, err := sinkHealthFrom(row.LastSuccessAt, row.LastFailureAt, row.LastError, row.Waiting, row.OldestWaiting)
		if err != nil {
			return nil, fmt.Errorf("read health of sink %d: %w", row.ID, err)
		}
		sinksOf[row.ConnectionID] = append(sinksOf[row.ConnectionID], h)
	}
	health := make(map[int64]SinkHealth, len(ids))
	for _, id := range ids {
		health[id] = summed(sinksOf[id])
	}
	return health, nil
}

// ForgeHealth reads the delivery health of the forge connections with ids,
// by connection id. A connection without any delivery has zero health.
func (p *Postgres) ForgeHealth(ctx context.Context, ids []int64) (map[int64]ForgeHealth, error) {
	raw, err := idsJSON(ids)
	if err != nil {
		return nil, err
	}
	rows, err := p.base.ConnectionHealthList(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("read forge connection health: %w", err)
	}
	health := make(map[int64]ForgeHealth, len(ids))
	for _, id := range ids {
		health[id] = ForgeHealth{}
	}
	for _, row := range rows {
		health[row.ConnectionID] = ForgeHealth{
			LastDelivery:   row.LastDeliveryAt.Time,
			LastRejection:  row.LastRejectionAt.Time,
			RejectionCause: row.LastRejection.String,
		}
	}
	return health, nil
}

// SinkQueues reads the queue of every sink, for the waiting gauges.
func (p *Postgres) SinkQueues(ctx context.Context) ([]SinkQueue, error) {
	rows, err := p.base.SinkQueues(ctx)
	if err != nil {
		return nil, fmt.Errorf("read sink queues: %w", err)
	}
	queues := make([]SinkQueue, 0, len(rows))
	for _, row := range rows {
		oldest, err := timeOrZero(row.OldestWaiting)
		if err != nil {
			return nil, fmt.Errorf("read queue of sink %d: %w", row.ID, err)
		}
		queues = append(queues, SinkQueue{
			SinkID:        row.ID,
			ConnectionID:  row.ConnectionID,
			Waiting:       row.Waiting,
			OldestWaiting: oldest,
		})
	}
	return queues, nil
}
