package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/patchcaster-app/patchcaster/internal/event"
	"github.com/patchcaster-app/patchcaster/internal/metrics"
	"github.com/patchcaster-app/patchcaster/internal/plan"
	"github.com/patchcaster-app/patchcaster/internal/store"
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
	Destinations(ctx context.Context, connectionID int64, repository string) ([]store.Destination, error)
	Accept(ctx context.Context, ev *event.Event, deliveries []store.Delivery, now time.Time) (store.AcceptResult, error)
	RecordForgeDelivery(ctx context.Context, connectionID int64, at time.Time) error
	RecordForgeRejection(ctx context.Context, connectionID int64, at time.Time, cause string) error
}

// Keys opens the signing keys of a forge connection. Keyring is the real one.
type Keys interface {
	SigningKeys(ctx context.Context, connectionID int64) ([][]byte, error)
}

// Handler serves the webhook endpoints of the forge connections.
type Handler struct {
	Keys    Keys
	Store   Store
	MaxAge  time.Duration    // freshness cap (SYSREQ-013)
	Now     func() time.Time // injectable clock
	Log     *slog.Logger
	Metrics *metrics.Metrics
}

// EndpointURL is where a forge sends the webhooks of the connection with
// connectionID: below the webhook listener's public address publicURL, by
// the connection's id.
func EndpointURL(publicURL *url.URL, connectionID int64) string {
	return publicURL.JoinPath("webhooks", strconv.FormatInt(connectionID, 10)).String()
}

// Mux exposes the endpoint surface for the public listener: one endpoint
// per forge connection, by its id (SYSREQ-044).
func (h *Handler) Mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/{connection}", h.handle)
	return mux
}

// handle applies the decision order. Every response is one of:
// 404 no such endpoint, 401 unauthenticated, 400 unparseable, 503 store
// unavailable, or 200 with the delivery durably settled (accepted,
// suppressed, or discarded).
func (h *Handler) handle(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("Webhook-Id")
	ts := r.Header.Get("Webhook-Timestamp")
	sig := r.Header.Get("Webhook-Signature")
	log := h.Log.With("webhook_id", id)

	// Until the endpoint names a known connection, the delivery counts
	// under no connection, so that the labels stay bounded.
	connectionID, keys, err := h.endpointKeys(r)
	if errors.Is(err, ErrUnknownEndpoint) {
		h.Metrics.CountReceived(metrics.NoConnection)
		h.reject(w, log, metrics.NoConnection, metrics.CauseEndpoint, http.StatusNotFound, "no forge connection at this endpoint")
		return
	}
	if err != nil {
		h.Metrics.CountReceived(metrics.NoConnection)
		log.Error("signing key lookup failed", "err", err)
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	connection := metrics.ConnectionOf(connectionID)
	h.Metrics.CountReceived(connection)
	log = log.With("connection", connectionID)

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		h.rejectFrom(w, r, log, connectionID, metrics.CauseParse, http.StatusBadRequest, "read body")
		return
	}

	// The identifier is part of the signed content, so a missing header fails
	// verification by construction (SYSREQ-012).
	if !Verify(keys, id, ts, body, sig) {
		h.rejectFrom(w, r, log, connectionID, metrics.CauseSignature, http.StatusUnauthorized, "signature verification")
		return
	}
	h.recordDelivery(r, log, connectionID)

	seen, err := h.Store.Seen(r.Context(), id)
	if err != nil {
		log.Error("dedupe lookup failed", "err", err)
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	if seen {
		h.Metrics.CountSuppressed(connection)
		log.Info("duplicate delivery suppressed")
		w.WriteHeader(http.StatusOK)
		return
	}

	ev, err := event.Parse(body)
	if err != nil {
		h.rejectFrom(w, r, log, connectionID, metrics.CauseParse, http.StatusBadRequest, "parse payload")
		return
	}
	ev.WebhookID = id
	onConnection(ev, connectionID)
	log = log.With("project", ev.ProjectPath, "mr", ev.MRRef, "action", ev.Action)

	if !ev.Processed() {
		h.discard(w, r, log, connection, id, metrics.CauseUnprocessed, "event kind not processed")
		return
	}

	// Per-kind admission (SYSREQ-008): merge requests gate on the action
	// table, comments and pipelines must name their merge request, pushes
	// must name a branch (tags attribute to nothing, SYSREQ-030).
	switch ev.Kind {
	case event.KindMergeRequest:
		if !acceptedActions[ev.Action] {
			h.discard(w, r, log, connection, id, metrics.CauseNotAllowlisted, "action not allowlisted")
			return
		}
	case event.KindNote, event.KindPipeline:
		if ev.MRRef == "" {
			h.discard(w, r, log, connection, id, metrics.CauseUnlinked, "delivery names no merge request")
			return
		}
	case event.KindPush:
		if ev.PushBranch == "" {
			h.discard(w, r, log, connection, id, metrics.CauseUnlinked, "push names no branch")
			return
		}
	}

	// Unparseable or absent timestamps leave When zero, which is older than
	// any real cap: treated as expired per SYSREQ-013 acceptance criteria.
	if h.Now().Add(-h.MaxAge).After(ev.When) {
		h.discard(w, r, log, connection, id, metrics.CauseExpired, "event older than freshness cap")
		return
	}

	ev.Plan = plan.For(ev)
	deliveries, err := h.deliveries(r.Context(), ev)
	if err != nil {
		log.Error("route lookup failed", "err", err)
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	if len(deliveries) == 0 {
		h.discard(w, r, log, connection, id, metrics.CauseUnrouted, "no route for project")
		return
	}

	now := h.Now()
	res, err := h.Store.Accept(r.Context(), ev, deliveries, now)
	if err != nil {
		log.Error("durable accept failed", "err", err)
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	if !res.Accepted {
		// A concurrent duplicate won the seen-insert race.
		h.Metrics.CountSuppressed(connection)
		log.Info("duplicate delivery suppressed at accept")
		w.WriteHeader(http.StatusOK)
		return
	}
	h.Metrics.CountStateApplied(ev.Kind, connection)
	if ev.Kind == event.KindPush && res.Attributed == 0 {
		h.Metrics.CountPushAttributionMiss(connection)
	}
	log.Info("delivery accepted", "kind", ev.Kind, "sinks", len(deliveries), "enqueued", res.Enqueued)
	w.WriteHeader(http.StatusOK)
}

// endpointKeys reads which forge connection's endpoint the request reached,
// and that connection's signing keys.
func (h *Handler) endpointKeys(r *http.Request) (int64, [][]byte, error) {
	connectionID, err := strconv.ParseInt(r.PathValue("connection"), 10, 64)
	if err != nil {
		return 0, nil, ErrUnknownEndpoint
	}
	keys, err := h.Keys.SigningKeys(r.Context(), connectionID)
	return connectionID, keys, err
}

// onConnection ties an event to the forge connection it came through. The
// forge's own merge request id is unique only on that forge, so the key the
// store and the queue use names the connection too (ADR-036).
func onConnection(ev *event.Event, connectionID int64) {
	ev.ConnectionID = connectionID
	if ev.MRRef != "" {
		ev.MRRef = strconv.FormatInt(connectionID, 10) + ":" + ev.MRRef
	}
}

// deliveries resolves the sinks the event reaches through the routes of the
// sources whose scope holds its repository (SYSREQ-046). A sink that
// several routes reach gets the event once, and its reply unless every one
// of those routes mutes it (SYSREQ-047). The route with the narrowest source
// writes its message with its templates (SYSREQ-049).
func (h *Handler) deliveries(ctx context.Context, ev *event.Event) ([]store.Delivery, error) {
	destinations, err := h.Store.Destinations(ctx, ev.ConnectionID, ev.ProjectPath)
	if err != nil {
		return nil, err
	}
	var deliveries []store.Delivery
	mutedBySink := map[int64][]string{}
	writerOfSink := map[int64]store.Destination{}
	for _, d := range destinations {
		muted, seen := mutedBySink[d.SinkID]
		if !seen {
			deliveries = append(deliveries, store.Delivery{
				SinkID:       d.SinkID,
				ConnectionID: d.ConnectionID,
				ChannelID:    d.ChannelID,
			})
			mutedBySink[d.SinkID] = d.MutedReplies
			writerOfSink[d.SinkID] = d
			continue
		}
		mutedBySink[d.SinkID] = mutedByAll(muted, d.MutedReplies)
		if isNarrower(d, writerOfSink[d.SinkID]) {
			writerOfSink[d.SinkID] = d
		}
	}
	for i := range deliveries {
		sinkID := deliveries[i].SinkID
		deliveries[i].Plan = plan.ForSink(ev.Plan, mutedBySink[sinkID])
		deliveries[i].RouteID = writerOfSink[sinkID].RouteID
	}
	return deliveries, nil
}

// scopeKindBreadth orders the kinds of source scopes from narrow to broad.
var scopeKindBreadth = map[string]int{
	"repository": 0,
	"group":      1,
	"instance":   2,
}

// isNarrower reports whether the source of route a is narrower than that of
// route b (owner decision 2026-10-20): a repository is narrower than a
// group, a group narrower than the instance, and a group narrower than the
// groups above it. Routes of equal sources are never narrower than one
// another, so the first route, the oldest, keeps a sink's message.
func isNarrower(a, b store.Destination) bool {
	aBreadth, bBreadth := scopeKindBreadth[a.ScopeKind], scopeKindBreadth[b.ScopeKind]
	if aBreadth != bBreadth {
		return aBreadth < bBreadth
	}
	return pathDepth(a.ScopePath) > pathDepth(b.ScopePath)
}

// pathDepth counts the segments of a scope's path.
func pathDepth(path string) int {
	if path == "" {
		return 0
	}
	return strings.Count(path, "/") + 1
}

// mutedByAll keeps the reply kinds that both routes mute.
func mutedByAll(a, b []string) []string {
	both := []string{}
	for _, kind := range a {
		if slices.Contains(b, kind) {
			both = append(both, kind)
		}
	}
	return both
}

// discard settles a verified delivery as seen-without-posting. The seen mark
// and its reason must persist, otherwise GitLab retries would churn the same
// verdict forever (SYSREQ-014).
func (h *Handler) discard(w http.ResponseWriter, r *http.Request, log *slog.Logger, connection metrics.Connection, id, cause, msg string) {
	if err := h.Store.MarkOutcome(r.Context(), id, cause, h.Now()); err != nil {
		log.Error("marking discard outcome failed", "err", err, "cause", cause)
		http.Error(w, "store unavailable", http.StatusServiceUnavailable)
		return
	}
	h.Metrics.CountDiscarded(cause, connection)
	log.Info("delivery discarded", "cause", cause, "detail", msg)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) reject(w http.ResponseWriter, log *slog.Logger, connection metrics.Connection, cause string, status int, msg string) {
	h.Metrics.CountRejected(cause, connection)
	log.Warn("delivery rejected", "cause", cause, "detail", msg)
	http.Error(w, http.StatusText(status), status)
}

// rejectFrom rejects a delivery to a known forge connection, whose health
// then shows the rejection (SYSREQ-064).
func (h *Handler) rejectFrom(w http.ResponseWriter, r *http.Request, log *slog.Logger, connectionID int64, cause string, status int, msg string) {
	if err := h.Store.RecordForgeRejection(r.Context(), connectionID, h.Now(), cause); err != nil {
		log.Warn("recording the rejection in the connection's health failed", "err", err)
	}
	h.reject(w, log, metrics.ConnectionOf(connectionID), cause, status, msg)
}

// recordDelivery shows an authentic delivery in its connection's health
// (SYSREQ-064). Health is no reason to refuse a delivery, so a failure is
// only logged.
func (h *Handler) recordDelivery(r *http.Request, log *slog.Logger, connectionID int64) {
	if err := h.Store.RecordForgeDelivery(r.Context(), connectionID, h.Now()); err != nil {
		log.Warn("recording the delivery in the connection's health failed", "err", err)
	}
}
