package ingest

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/0x5a17ed/patchcaster/internal/event"
	"github.com/0x5a17ed/patchcaster/internal/metrics"
	"github.com/0x5a17ed/patchcaster/internal/plan"
	"github.com/0x5a17ed/patchcaster/internal/routing"
	"github.com/0x5a17ed/patchcaster/internal/store"
)

// maxBodyBytes bounds one webhook payload; GitLab merge request payloads stay
// well below this, and the cap keeps memory use predictable.
const maxBodyBytes = 4 << 20

// acceptedActions is the deny-by-default action table for merge request
// deliveries (SYSREQ-008). Recognized actions are processed — silent ones
// still refresh board state — anything else is acknowledged and discarded.
var acceptedActions = map[string]bool{
	"open":       true,
	"close":      true,
	"reopen":     true,
	"update":     true,
	"approval":   true,
	"approved":   true,
	"unapproval": true,
	"unapproved": true,
	"merge":      true,
}

// Store is what the handler needs from persistence, kept narrow so the
// decision table is testable without a database.
type Store interface {
	Seen(ctx context.Context, webhookID string) (bool, error)
	MarkOutcome(ctx context.Context, webhookID, outcome string, now time.Time) error
	Accept(ctx context.Context, ev *event.Event, destChannel string, now time.Time) (store.AcceptResult, error)
}

// Handler serves the webhook endpoint.
type Handler struct {
	Secrets [][]byte
	Store   Store
	Routes  *routing.Table
	MaxAge  time.Duration    // freshness cap (SYSREQ-013)
	Now     func() time.Time // injectable clock
	Log     *slog.Logger
	Metrics *metrics.Metrics
}

// Mux exposes the endpoint surface for the public listener.
func (h *Handler) Mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhook", h.handle)
	return mux
}

// handle applies the decision order. Every response is one of:
// 401 unauthenticated, 400 unparseable, 503 store unavailable, or 200 with
// the delivery durably settled (accepted, suppressed, or discarded).
func (h *Handler) handle(w http.ResponseWriter, r *http.Request) {
	h.Metrics.Received.Inc()

	id := r.Header.Get("Webhook-Id")
	ts := r.Header.Get("Webhook-Timestamp")
	sig := r.Header.Get("Webhook-Signature")
	log := h.Log.With("webhook_id", id)

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		h.reject(w, log, metrics.CauseParse, http.StatusBadRequest, "read body")
		return
	}

	// The identifier is part of the signed content, so a missing header fails
	// verification by construction (SYSREQ-012).
	if !Verify(h.Secrets, id, ts, body, sig) {
		h.reject(w, log, metrics.CauseSignature, http.StatusUnauthorized, "signature verification")
		return
	}

	seen, err := h.Store.Seen(r.Context(), id)
	if err != nil {
		log.Error("dedupe lookup failed", "err", err)
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	if seen {
		h.Metrics.Suppressed.Inc()
		log.Info("duplicate delivery suppressed")
		w.WriteHeader(http.StatusOK)
		return
	}

	ev, err := event.Parse(body)
	if err != nil {
		h.reject(w, log, metrics.CauseParse, http.StatusBadRequest, "parse payload")
		return
	}
	ev.WebhookID = id
	log = log.With("project", ev.ProjectPath, "mr", ev.MRRef, "action", ev.Action)

	if !ev.Processed() {
		h.discard(w, r, log, id, metrics.CauseUnprocessed, "event kind not processed")
		return
	}

	// Per-kind admission (SYSREQ-008): merge requests gate on the action
	// table, comments and pipelines must name their merge request, pushes
	// must name a branch (tags attribute to nothing, SYSREQ-030).
	switch ev.Kind {
	case event.KindMergeRequest:
		if !acceptedActions[ev.Action] {
			h.discard(w, r, log, id, metrics.CauseNotAllowlisted, "action not allowlisted")
			return
		}
	case event.KindNote, event.KindPipeline:
		if ev.MRRef == "" {
			h.discard(w, r, log, id, metrics.CauseUnlinked, "delivery names no merge request")
			return
		}
	case event.KindPush:
		if ev.PushBranch == "" {
			h.discard(w, r, log, id, metrics.CauseUnlinked, "push names no branch")
			return
		}
	}

	// Unparseable or absent timestamps leave When zero, which is older than
	// any real cap: treated as expired per SYSREQ-013 acceptance criteria.
	if h.Now().Add(-h.MaxAge).After(ev.When) {
		h.discard(w, r, log, id, metrics.CauseExpired, "event older than freshness cap")
		return
	}

	dest, ok := h.Routes.Channel(ev.ProjectPath)
	if !ok {
		h.discard(w, r, log, id, metrics.CauseUnrouted, "no route for project")
		return
	}

	ev.Plan = plan.For(ev)
	now := h.Now()
	res, err := h.Store.Accept(r.Context(), ev, dest, now)
	if err != nil {
		log.Error("durable accept failed", "err", err)
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	if !res.Accepted {
		// A concurrent duplicate won the seen-insert race.
		h.Metrics.Suppressed.Inc()
		log.Info("duplicate delivery suppressed at accept")
		w.WriteHeader(http.StatusOK)
		return
	}
	h.Metrics.StateApplied.WithLabelValues(ev.Kind).Inc()
	if ev.Kind == event.KindPush && res.Attributed == 0 {
		h.Metrics.PushAttrMiss.Inc()
	}
	log.Info("delivery accepted", "kind", ev.Kind, "enqueued", res.Enqueued)
	w.WriteHeader(http.StatusOK)
}

// discard settles a verified delivery as seen-without-posting. The seen mark
// and its reason must persist, otherwise GitLab retries would churn the same
// verdict forever (SYSREQ-014).
func (h *Handler) discard(w http.ResponseWriter, r *http.Request, log *slog.Logger, id, cause, msg string) {
	if err := h.Store.MarkOutcome(r.Context(), id, cause, h.Now()); err != nil {
		log.Error("marking discard outcome failed", "err", err, "cause", cause)
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	h.Metrics.Discarded.WithLabelValues(cause).Inc()
	log.Info("delivery discarded", "cause", cause, "detail", msg)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) reject(w http.ResponseWriter, log *slog.Logger, cause string, status int, msg string) {
	h.Metrics.Rejected.WithLabelValues(cause).Inc()
	log.Warn("delivery rejected", "cause", cause, "detail", msg)
	http.Error(w, http.StatusText(status), status)
}
