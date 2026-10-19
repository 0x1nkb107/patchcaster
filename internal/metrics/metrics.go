// Package metrics defines the ingest-to-post funnel counters (SYSREQ-019),
// labelled by connection and sink (SYSREQ-064), the waiting gauges of the
// sinks, and the wake-up listener gauge (ADR-019). Names, labels and label
// causes live here and only here.
package metrics

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Rejection causes (HTTP 404/401/400 paths).
const (
	CauseEndpoint  = "endpoint"
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
	CauseSinkGone      = "sink_gone"
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

// Label names. Connections and sinks appear by their ids, never by a name,
// so that no secret or personal value reaches a label, and their number
// follows the configuration, not the traffic (SYSREQ-064).
const (
	labelConnection = "connection"
	labelSink       = "sink"
)

// queueReadTimeout bounds how long a scrape waits for the sink queues.
const queueReadTimeout = 2 * time.Second

// Connection names the forge connection a delivery reached.
type Connection struct {
	label string
}

// NoConnection names no connection, for a delivery whose endpoint names
// none. The label stays bounded however many unknown endpoints are tried.
var NoConnection = Connection{}

// ConnectionOf names the forge connection with id.
func ConnectionOf(id int64) Connection {
	return Connection{label: strconv.FormatInt(id, 10)}
}

// Sink names the sink a message goes to and the chat connection that
// delivers it.
type Sink struct {
	connection, sink string
}

// SinkOf names the sink with sinkID of the chat connection with
// connectionID.
func SinkOf(connectionID, sinkID int64) Sink {
	return Sink{
		connection: strconv.FormatInt(connectionID, 10),
		sink:       strconv.FormatInt(sinkID, 10),
	}
}

// Metrics is the single wrapper over the Prometheus registry; nothing outside
// this package declares metric names (SYSARCH-004).
type Metrics struct {
	Registry *prometheus.Registry

	Received   *prometheus.CounterVec
	Rejected   *prometheus.CounterVec
	Suppressed *prometheus.CounterVec
	Discarded  *prometheus.CounterVec
	Skipped    *prometheus.CounterVec
	Parked     *prometheus.CounterVec
	Posted     *prometheus.CounterVec
	MMAttempts *prometheus.CounterVec
	MMDuration *prometheus.HistogramVec

	// Live-state board funnels (SYSREQ-019, amended 2026-10-10): state
	// applications by kind, thread notices by kind, and push deliveries whose
	// project+branch matched no open merge request (SYSREQ-030 attribution
	// misses — fork pushes and pushes before an MR is announced).
	StateApplied *prometheus.CounterVec
	Notices      *prometheus.CounterVec
	PushAttrMiss *prometheus.CounterVec

	// TemplateFallbacks counts messages that a route's own template failed
	// to write, so that the built-in template wrote them (SYSREQ-049).
	TemplateFallbacks *prometheus.CounterVec

	// ListenerUp is 1 while a confirmed session listens for new outbox rows
	// and 0 while the worker relies on polling alone (ADR-019).
	ListenerUp prometheus.Gauge
}

// New registers the funnel on a private registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg,
		Received: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_deliveries_received_total",
			Help: "Webhook deliveries received on the ingest endpoint, by forge connection.",
		}, []string{labelConnection}),
		Rejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_deliveries_rejected_total",
			Help: "Deliveries rejected before content decisions, by cause and forge connection.",
		}, []string{"cause", labelConnection}),
		Suppressed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_deliveries_suppressed_total",
			Help: "Duplicate deliveries suppressed by seen webhook identifier, by forge connection.",
		}, []string{labelConnection}),
		Discarded: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_deliveries_discarded_total",
			Help: "Authentic deliveries discarded without posting, by cause and forge connection.",
		}, []string{"cause", labelConnection}),
		Skipped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_messages_skipped_total",
			Help: "Queued messages skipped by the worker, by cause, chat connection and sink.",
		}, []string{"cause", labelConnection, labelSink}),
		Parked: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_messages_parked_total",
			Help: "Messages parked, by cause, chat connection and sink.",
		}, []string{"cause", labelConnection, labelSink}),
		Posted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_messages_posted_total",
			Help: "Messages accepted by Mattermost, by chat connection and sink.",
		}, []string{labelConnection, labelSink}),
		MMAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_mattermost_attempts_total",
			Help: "Mattermost delivery attempts by outcome, chat connection and sink.",
		}, []string{"outcome", labelConnection, labelSink}),
		MMDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "patchcaster_mattermost_attempt_duration_seconds",
			Help:    "Duration of Mattermost delivery attempts, by chat connection.",
			Buckets: prometheus.DefBuckets,
		}, []string{labelConnection}),
		StateApplied: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_state_applied_total",
			Help: "Accepted deliveries whose board state was applied, by kind and forge connection.",
		}, []string{"kind", labelConnection}),
		Notices: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_notices_total",
			Help: "Thread notices posted, by notice kind.",
		}, []string{"notice"}),
		PushAttrMiss: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_push_attribution_misses_total",
			Help: "Push deliveries that matched no open merge request, by forge connection.",
		}, []string{labelConnection}),
		TemplateFallbacks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "patchcaster_template_fallbacks_total",
			Help: "Messages written with the built-in template because the route's own failed, by kind, chat connection and sink.",
		}, []string{"kind", labelConnection, labelSink}),
		ListenerUp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "patchcaster_outbox_listener_up",
			Help: "1 while the worker is woken for new messages, 0 while it only polls.",
		}),
	}
	reg.MustRegister(
		m.Received, m.Rejected, m.Suppressed, m.Discarded, m.Skipped,
		m.Parked, m.Posted, m.MMAttempts, m.MMDuration,
		m.StateApplied, m.Notices, m.PushAttrMiss, m.TemplateFallbacks,
		m.ListenerUp,
	)
	return m
}

// CountReceived counts a delivery that reached the endpoint of c.
func (m *Metrics) CountReceived(c Connection) {
	m.Received.WithLabelValues(c.label).Inc()
}

// CountRejected counts a delivery to c rejected for cause.
func (m *Metrics) CountRejected(cause string, c Connection) {
	m.Rejected.WithLabelValues(cause, c.label).Inc()
}

// CountSuppressed counts a duplicate delivery to c.
func (m *Metrics) CountSuppressed(c Connection) {
	m.Suppressed.WithLabelValues(c.label).Inc()
}

// CountDiscarded counts an authentic delivery to c discarded for cause.
func (m *Metrics) CountDiscarded(cause string, c Connection) {
	m.Discarded.WithLabelValues(cause, c.label).Inc()
}

// CountStateApplied counts an accepted delivery of kind to c.
func (m *Metrics) CountStateApplied(kind string, c Connection) {
	m.StateApplied.WithLabelValues(kind, c.label).Inc()
}

// CountPushAttributionMiss counts a push to c that matched no open merge
// request.
func (m *Metrics) CountPushAttributionMiss(c Connection) {
	m.PushAttrMiss.WithLabelValues(c.label).Inc()
}

// CountTemplateFallback counts a message of kind for s that a route's own
// template failed to write, so that the built-in template wrote it.
func (m *Metrics) CountTemplateFallback(kind string, s Sink) {
	m.TemplateFallbacks.WithLabelValues(kind, s.connection, s.sink).Inc()
}

// CountSkipped counts a message for s skipped for cause.
func (m *Metrics) CountSkipped(cause string, s Sink) {
	m.Skipped.WithLabelValues(cause, s.connection, s.sink).Inc()
}

// CountParked counts a message for s parked for cause.
func (m *Metrics) CountParked(cause string, s Sink) {
	m.Parked.WithLabelValues(cause, s.connection, s.sink).Inc()
}

// CountPosted counts a message the platform of s accepted.
func (m *Metrics) CountPosted(s Sink) {
	m.Posted.WithLabelValues(s.connection, s.sink).Inc()
}

// CountAttempt counts an attempt at the platform of s, its outcome and how
// long it took.
func (m *Metrics) CountAttempt(outcome string, s Sink, took time.Duration) {
	m.MMAttempts.WithLabelValues(outcome, s.connection, s.sink).Inc()
	m.MMDuration.WithLabelValues(s.connection).Observe(took.Seconds())
}

// SinkQueue is the queue of one sink: its waiting messages and when the
// oldest of them was queued, zero when none waits.
type SinkQueue struct {
	ConnectionID  int64
	SinkID        int64
	Waiting       int64
	OldestWaiting time.Time
}

// QueueReader reads the queue of every sink.
type QueueReader func(ctx context.Context) ([]SinkQueue, error)

// RegisterQueues adds the waiting gauges of the sinks. They read the queues
// with read at every scrape, so every replica reports the shared queue as
// it is (SYSREQ-064).
func (m *Metrics) RegisterQueues(read QueueReader, now func() time.Time, log *slog.Logger) {
	m.Registry.MustRegister(&queueCollector{
		read: read,
		now:  now,
		log:  log,
		waiting: prometheus.NewDesc("patchcaster_sink_waiting_messages",
			"Messages queued for a sink and not yet delivered, by chat connection and sink.",
			[]string{labelConnection, labelSink}, nil),
		oldest: prometheus.NewDesc("patchcaster_sink_oldest_waiting_seconds",
			"Age of the oldest message waiting for a sink, 0 when none waits, by chat connection and sink.",
			[]string{labelConnection, labelSink}, nil),
	})
}

// queueCollector reports the waiting gauges from the queues it reads.
type queueCollector struct {
	read            QueueReader
	now             func() time.Time
	log             *slog.Logger
	waiting, oldest *prometheus.Desc
}

func (c *queueCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.waiting
	ch <- c.oldest
}

// Collect reports nothing when the queues cannot be read, so that a store
// outage fails no scrape of the other metrics.
func (c *queueCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), queueReadTimeout)
	defer cancel()
	queues, err := c.read(ctx)
	if err != nil {
		c.log.Warn("read sink queues failed", "err", err)
		return
	}
	now := c.now()
	for _, q := range queues {
		s := SinkOf(q.ConnectionID, q.SinkID)
		ch <- prometheus.MustNewConstMetric(c.waiting, prometheus.GaugeValue, float64(q.Waiting), s.connection, s.sink)
		ch <- prometheus.MustNewConstMetric(c.oldest, prometheus.GaugeValue, ageOf(q.OldestWaiting, now), s.connection, s.sink)
	}
}

// ageOf returns the seconds since queued, 0 for no message.
func ageOf(queued, now time.Time) float64 {
	if queued.IsZero() {
		return 0
	}
	return max(now.Sub(queued).Seconds(), 0)
}
