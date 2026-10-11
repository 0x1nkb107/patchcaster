// Package metrics defines the ingest-to-post funnel counters (SYSREQ-019)
// and the wake-up listener gauge (ADR-019). Names and label causes live here
// and only here.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Rejection causes (HTTP 401/400 paths).
const (
	CauseSignature = "signature"
	CauseParse     = "parse"
)

// Discard causes (authentic deliveries with no post).
const (
	CauseExpired        = "expired"
	CauseUnrouted       = "unrouted"
	CauseNotAllowlisted = "not_allowlisted"
	CauseUnprocessed    = "unprocessed_kind"
	CauseUnlinked       = "unlinked"
)

// Skip causes (queued messages the worker must not post).
const (
	CauseNoMapping     = "no_mapping"
	CauseThreadClosed  = "thread_closed"
	CauseDuplicateOpen = "duplicate_open"
)

// Park causes.
const (
	CauseThreadGone   = "thread_gone"
	CauseBudgetExched = "attempt_budget"
)

// Mattermost attempt outcomes.
const (
	OutcomeSent        = "sent"
	OutcomeRetryable   = "retryable"
	OutcomeRateLimited = "ratelimited"
	OutcomeThreadGone  = "thread_gone"
	OutcomeTransport   = "transport"
)

// Metrics is the single wrapper over the Prometheus registry; nothing outside
// this package declares metric names (SYSARCH-004).
type Metrics struct {
	Registry *prometheus.Registry

	Received   prometheus.Counter
	Rejected   *prometheus.CounterVec
	Suppressed prometheus.Counter
	Discarded  *prometheus.CounterVec
	Skipped    *prometheus.CounterVec
	Parked     *prometheus.CounterVec
	Posted     prometheus.Counter
	MMAttempts *prometheus.CounterVec
	MMDuration prometheus.Histogram

	// Live-state board funnels (SYSREQ-019, amended 2026-10-10): state
	// applications by kind, thread notices by kind, and push deliveries whose
	// project+branch matched no open merge request (SYSREQ-030 attribution
	// misses — fork pushes and pushes before an MR is announced).
	StateApplied *prometheus.CounterVec
	Notices      *prometheus.CounterVec
	PushAttrMiss prometheus.Counter

	// ListenerUp is 1 while a confirmed session listens for new outbox rows
	// and 0 while the worker relies on polling alone (ADR-019).
	ListenerUp prometheus.Gauge
}

// New registers the funnel on a private registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg,
		Received: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "patchcaster_deliveries_received_total",
			Help: "Webhook deliveries received on the ingest endpoint.",
		}),
		Rejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_deliveries_rejected_total",
			Help: "Deliveries rejected before content decisions, by cause.",
		}, []string{"cause"}),
		Suppressed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "patchcaster_deliveries_suppressed_total",
			Help: "Duplicate deliveries suppressed by seen webhook identifier.",
		}),
		Discarded: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_deliveries_discarded_total",
			Help: "Authentic deliveries discarded without posting, by cause.",
		}, []string{"cause"}),
		Skipped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_messages_skipped_total",
			Help: "Queued messages skipped by the worker, by cause.",
		}, []string{"cause"}),
		Parked: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_messages_parked_total",
			Help: "Messages parked, by cause.",
		}, []string{"cause"}),
		Posted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "patchcaster_messages_posted_total",
			Help: "Messages accepted by Mattermost.",
		}),
		MMAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_mattermost_attempts_total",
			Help: "Mattermost delivery attempts by outcome.",
		}, []string{"outcome"}),
		MMDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "patchcaster_mattermost_attempt_duration_seconds",
			Help:    "Duration of Mattermost delivery attempts.",
			Buckets: prometheus.DefBuckets,
		}),
		StateApplied: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_state_applied_total",
			Help: "Accepted deliveries whose board state was applied, by kind.",
		}, []string{"kind"}),
		Notices: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_notices_total",
			Help: "Thread notices posted, by notice kind.",
		}, []string{"notice"}),
		PushAttrMiss: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "patchcaster_push_attribution_misses_total",
			Help: "Push deliveries that matched no open merge request.",
		}),
		ListenerUp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "patchcaster_outbox_listener_up",
			Help: "1 while the worker is woken for new messages, 0 while it only polls.",
		}),
	}
	reg.MustRegister(
		m.Received, m.Rejected, m.Suppressed, m.Discarded, m.Skipped,
		m.Parked, m.Posted, m.MMAttempts, m.MMDuration,
		m.StateApplied, m.Notices, m.PushAttrMiss,
		m.ListenerUp,
	)
	return m
}
