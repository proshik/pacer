// Package metrics holds the Prometheus metrics of pacer and the scrape
// endpoint Krill's collector reads.
//
// Every recording method is safe on a nil *Metrics and does nothing, so the
// rest of the code records unconditionally and a deployment without
// METRICS_ADDR behaves exactly as before.
//
// Labels carry only values from fixed sets: route patterns, methods, status
// codes, update types and queue names. User ids, plan text and the bot token
// never become a label.
package metrics

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "pacer"

// Telegram update types, the values of the type label.
const (
	UpdateMessage       = "message"
	UpdateCallbackQuery = "callback_query"
	UpdateInlineQuery   = "inline_query"
)

// Telegram queues, the values of the queue label of the drop counter.
const (
	QueueUpdates  = "updates"
	QueueOutgoing = "outgoing"
)

// Saved-runs operations and their outcomes.
const (
	HistoryList   = "list"
	HistorySave   = "save"
	HistoryDelete = "delete"

	ResultOK    = "ok"
	ResultError = "error"
)

// Reply kinds, the values of the reply label of the send error counter.
var replyKinds = []string{"text", "photo", "greeting", "callback", "inline", "other"}

// Metrics is the registry of pacer metrics. It owns its registry instead of
// the global default one, so nothing a dependency registers behind our back
// shows up in the scrape.
type Metrics struct {
	registry *prometheus.Registry

	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec

	botUpdates    *prometheus.CounterVec
	botSendErrors *prometheus.CounterVec
	botDropped    *prometheus.CounterVec

	historyOps *prometheus.CounterVec
}

// New builds the registry with the Go runtime and process collectors and the
// pacer metrics. Known label combinations start at zero, so rate() and alerts
// see a series before the first event.
func New() (*Metrics, error) {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "HTTP requests by route pattern, method and status code.",
		}, []string{"route", "method", "code"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration by route pattern and method.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"route", "method"}),
		botUpdates: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "telegram",
			Name:      "updates_total",
			Help:      "Telegram updates handled by the bot, by update type.",
		}, []string{"type"}),
		botSendErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "telegram",
			Name:      "send_errors_total",
			Help:      "Replies Telegram did not accept, by reply kind.",
		}, []string{"reply"}),
		botDropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "telegram",
			Name:      "dropped_total",
			Help:      "Updates and replies dropped because their queue was full.",
		}, []string{"queue"}),
		historyOps: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "history",
			Name:      "operations_total",
			Help:      "Saved-runs database operations by operation and result.",
		}, []string{"operation", "result"}),
	}

	for _, collector := range []prometheus.Collector{
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.httpRequests,
		m.httpDuration,
		m.botUpdates,
		m.botSendErrors,
		m.botDropped,
		m.historyOps,
	} {
		if err := m.registry.Register(collector); err != nil {
			return nil, fmt.Errorf("register metrics collector: %w", err)
		}
	}

	for _, kind := range []string{UpdateMessage, UpdateCallbackQuery, UpdateInlineQuery} {
		m.botUpdates.WithLabelValues(kind)
	}
	for _, kind := range replyKinds {
		m.botSendErrors.WithLabelValues(kind)
	}
	for _, queue := range []string{QueueUpdates, QueueOutgoing} {
		m.botDropped.WithLabelValues(queue)
	}
	for _, operation := range []string{HistoryList, HistorySave, HistoryDelete} {
		for _, result := range []string{ResultOK, ResultError} {
			m.historyOps.WithLabelValues(operation, result)
		}
	}

	return m, nil
}

// Gatherer exposes the registry to tests and to the scrape handler.
func (m *Metrics) Gatherer() prometheus.Gatherer {
	return m.registry
}

// ObserveHTTP records one finished request. route must be a registered
// pattern, never the raw path.
func (m *Metrics) ObserveHTTP(route, method string, code int, elapsed time.Duration) {
	if m == nil {
		return
	}

	method = normalizeMethod(method)
	m.httpRequests.WithLabelValues(route, method, strconv.Itoa(code)).Inc()
	m.httpDuration.WithLabelValues(route, method).Observe(elapsed.Seconds())
}

// BotUpdate counts an update the bot handled; kind is one of the Update*
// constants.
func (m *Metrics) BotUpdate(kind string) {
	if m == nil {
		return
	}

	m.botUpdates.WithLabelValues(kind).Inc()
}

// BotSendError counts a reply Telegram rejected. Unknown kinds count as
// "other", so the label set stays fixed.
func (m *Metrics) BotSendError(kind string) {
	if m == nil {
		return
	}

	if !slices.Contains(replyKinds, kind) {
		kind = "other"
	}

	m.botSendErrors.WithLabelValues(kind).Inc()
}

// BotDropped counts an update or reply dropped on a full queue; queue is one
// of the Queue* constants.
func (m *Metrics) BotDropped(queue string) {
	if m == nil {
		return
	}

	m.botDropped.WithLabelValues(queue).Inc()
}

// HistoryOp counts a saved-runs database call. A delete of a missing run is
// ok: the database answered.
func (m *Metrics) HistoryOp(operation string, err error) {
	if m == nil {
		return
	}

	result := ResultOK
	if err != nil {
		result = ResultError
	}

	m.historyOps.WithLabelValues(operation, result).Inc()
}

// Handler serves the scrape. Krill's collector always sends
// "Authorization: Bearer <token>"; anything else, and every request while the
// token is empty, gets 401. The comparison is constant time.
func (m *Metrics) Handler(token string) http.Handler {
	scrape := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
	expected := []byte("Bearer " + token)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), expected) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		scrape.ServeHTTP(w, r)
	})
}

// normalizeMethod keeps the method label to the standard methods: any other
// verb a client invents is "other".
func normalizeMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	default:
		return "other"
	}
}
