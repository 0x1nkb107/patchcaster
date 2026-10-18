package api

import (
	"context"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/platform"
	"github.com/patchcaster-app/patchcaster/internal/store"
)

// optTime renders a time for the API, absent when it is zero.
func optTime(t time.Time) v1gen.OptDateTime {
	if t.IsZero() {
		return v1gen.OptDateTime{}
	}
	return v1gen.NewOptDateTime(t)
}

// optText renders a text for the API, absent when it is empty.
func optText(s string) v1gen.OptString {
	if s == "" {
		return v1gen.OptString{}
	}
	return v1gen.NewOptString(s)
}

func sinkHealthOf(h store.SinkHealth) v1gen.SinkHealth {
	return v1gen.SinkHealth{
		WaitingMessages:    h.Waiting,
		OldestWaitingSince: optTime(h.OldestWaiting),
		LastSuccessAt:      optTime(h.LastSuccess),
		LastFailureAt:      optTime(h.LastFailure),
		LastError:          optText(h.LastError),
	}
}

func forgeHealthOf(h store.ForgeHealth) v1gen.ConnectionHealth {
	health := v1gen.ConnectionHealth{
		LastDeliveryAt:  optTime(h.LastDelivery),
		LastRejectionAt: optTime(h.LastRejection),
	}
	if h.RejectionCause != "" {
		health.LastRejection = v1gen.NewOptConnectionHealthLastRejection(
			v1gen.ConnectionHealthLastRejection(h.RejectionCause))
	}
	return health
}

// chatHealthOf renders the summed health of a chat connection's sinks.
func chatHealthOf(h store.SinkHealth) v1gen.ConnectionHealth {
	return v1gen.ConnectionHealth{
		WaitingMessages:    v1gen.NewOptInt64(h.Waiting),
		OldestWaitingSince: optTime(h.OldestWaiting),
		LastSuccessAt:      optTime(h.LastSuccess),
		LastFailureAt:      optTime(h.LastFailure),
		LastError:          optText(h.LastError),
	}
}

// sinkHealth reads the health of sinks, by sink id (SYSREQ-064).
func (s *Service) sinkHealth(ctx context.Context, sinks []store.Sink) (map[int64]v1gen.SinkHealth, error) {
	ids := make([]int64, 0, len(sinks))
	for _, k := range sinks {
		ids = append(ids, k.ID)
	}
	read, err := s.store.SinkHealth(ctx, ids)
	if err != nil {
		return nil, err
	}
	health := make(map[int64]v1gen.SinkHealth, len(sinks))
	for _, k := range sinks {
		health[k.ID] = sinkHealthOf(read[k.ID])
	}
	return health, nil
}

// connectionHealth reads the health of connections, by connection id
// (SYSREQ-064): the deliveries of a forge connection, and the sinks of a
// chat connection summed up.
func (s *Service) connectionHealth(ctx context.Context, connections []store.Connection) (map[int64]v1gen.ConnectionHealth, error) {
	var forges, chats []int64
	for _, c := range connections {
		if platform.Kind(c.Kind).IsForge() {
			forges = append(forges, c.ID)
		} else {
			chats = append(chats, c.ID)
		}
	}
	forgeHealth, err := s.store.ForgeHealth(ctx, forges)
	if err != nil {
		return nil, err
	}
	chatHealth, err := s.store.ChatConnectionHealth(ctx, chats)
	if err != nil {
		return nil, err
	}
	health := make(map[int64]v1gen.ConnectionHealth, len(connections))
	for _, id := range forges {
		health[id] = forgeHealthOf(forgeHealth[id])
	}
	for _, id := range chats {
		health[id] = chatHealthOf(chatHealth[id])
	}
	return health, nil
}
