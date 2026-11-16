package rest

import (
	"net/http"
	"strings"
	"time"

	"gorun/pkg/metrics"
)

// webhookRoute stands for the webhook in the route label: its pattern is the
// bot token itself and must not reach the metrics.
const webhookRoute = "/{telegram_token}"

// unmatchedRoute labels requests no pattern took, such as the mux's own
// redirects, so the raw path never becomes a label.
const unmatchedRoute = "unmatched"

// instrument counts and times every request by the pattern the mux matched.
// ServeMux writes the matched pattern into r.Pattern of the very request it was
// given, so it is readable here after the call returns.
func instrument(next *http.ServeMux, m *metrics.Metrics, tgToken string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		m.ObserveHTTP(routeLabel(r.Pattern, tgToken), r.Method, recorder.status, time.Since(started))
	})
}

// routeLabel turns a matched pattern ("GET /api/v1/runs") into the route label
// ("/api/v1/runs"); the method is a label of its own. Any pattern holding the
// bot token becomes webhookRoute.
func routeLabel(pattern, tgToken string) string {
	if pattern == "" {
		return unmatchedRoute
	}

	if _, path, ok := strings.Cut(pattern, " "); ok {
		pattern = path
	}

	if tgToken != "" && strings.Contains(pattern, tgToken) {
		return webhookRoute
	}

	return pattern
}

// statusRecorder remembers the status code a handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true

	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
