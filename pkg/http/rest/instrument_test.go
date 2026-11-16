package rest

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"gorun/pkg/calculator"
	"gorun/pkg/history"
	"gorun/pkg/metrics"
)

func newMetrics(t *testing.T) *metrics.Metrics {
	t.Helper()

	m, err := metrics.New()
	if err != nil {
		t.Fatalf("create metrics: %v", err)
	}

	return m
}

// botToken looks like a real one: the webhook path is "/" + botToken.
const botToken = "123456789:AAE-secret_part"

func newInstrumentedHandler(t *testing.T, debugMode bool, store *history.Store) (http.Handler, *metrics.Metrics) {
	t.Helper()

	m := newMetrics(t)
	assets := fstest.MapFS{"assets/index.html": &fstest.MapFile{Data: []byte("<!doctype html>")}}

	handler, err := NewHandler(debugMode, botToken, "pacer.example.com", nil, calculator.NewService(), store, m, assets)
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}

	return handler, m
}

func scrapeMetrics(t *testing.T, m *metrics.Metrics) string {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, metrics.Path, nil)
	request.Header.Set("Authorization", "Bearer scrape")
	recorder := httptest.NewRecorder()
	m.Handler("scrape").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("scrape status = %d", recorder.Code)
	}

	return recorder.Body.String()
}

func TestRequestsAreCountedByRoutePattern(t *testing.T) {
	handler, m := newInstrumentedHandler(t, false, nil)

	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/api/v1/time?pace=5m&dist=10000"},
		{http.MethodGet, "/api/v1/time?pace=bad&dist=10000"},
		{http.MethodDelete, "/api/v1/runs/17"},
		{http.MethodDelete, "/api/v1/runs/18"},
		{http.MethodGet, "/no/such/file-12345.js"},
	} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(request.method, request.path, nil))
	}

	body := scrapeMetrics(t, m)

	for _, want := range []string{
		`pacer_http_requests_total{code="200",method="GET",route="/healthz"} 1`,
		`pacer_http_requests_total{code="200",method="GET",route="/api/v1/time"} 1`,
		`pacer_http_requests_total{code="400",method="GET",route="/api/v1/time"} 1`,
		// Both ids land on one series: the pattern, not the path.
		`pacer_http_requests_total{code="503",method="DELETE",route="/api/v1/runs/{id}"} 2`,
		`pacer_http_requests_total{code="404",method="GET",route="/"} 1`,
		`pacer_http_request_duration_seconds_count{method="GET",route="/api/v1/time"} 2`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape has no %q", want)
		}
	}

	for _, raw := range []string{"/runs/17", "/runs/18", "file-12345", "pace=", "dist="} {
		if strings.Contains(body, raw) {
			t.Errorf("a raw path or query %q leaked into the metrics", raw)
		}
	}
}

// The webhook pattern is the bot token. It must be labelled by a placeholder,
// and the token must not appear anywhere in the scrape.
func TestWebhookTokenNeverReachesMetrics(t *testing.T) {
	handler, m := newInstrumentedHandler(t, false, nil)

	request := httptest.NewRequest(http.MethodPost, "/"+botToken, strings.NewReader("not json"))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("webhook status = %d, want %d (is the route registered?)", recorder.Code, http.StatusBadRequest)
	}

	body := scrapeMetrics(t, m)

	if want := `pacer_http_requests_total{code="400",method="POST",route="/{telegram_token}"} 1`; !strings.Contains(body, want) {
		t.Errorf("scrape has no %q", want)
	}
	for _, secret := range []string{botToken, "123456789", "AAE-secret_part"} {
		if strings.Contains(body, secret) {
			t.Fatalf("the bot token (%q) leaked into the metrics", secret)
		}
	}
}

func TestRouteLabel(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{pattern: "", want: "unmatched"},
		{pattern: "/", want: "/"},
		{pattern: "/healthz", want: "/healthz"},
		{pattern: "GET /api/v1/runs", want: "/api/v1/runs"},
		{pattern: "DELETE /api/v1/runs/{id}", want: "/api/v1/runs/{id}"},
		{pattern: "GET /{$}", want: "/{$}"},
		{pattern: "/" + botToken, want: "/{telegram_token}"},
		{pattern: "POST /" + botToken, want: "/{telegram_token}"},
	}

	for _, tt := range tests {
		if got := routeLabel(tt.pattern, botToken); got != tt.want {
			t.Errorf("routeLabel(%q) = %q, want %q", tt.pattern, got, tt.want)
		}
	}
}

func TestHistoryOperationsAreCounted(t *testing.T) {
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	handler, m := newInstrumentedHandler(t, true, store)
	authorization := "tma " + signInitData(botToken, time.Now(), `{"id":42,"first_name":"Runner"}`)

	for _, request := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/runs", `{"distance":10000,"time_seconds":3000}`},
		{http.MethodGet, "/api/v1/runs", ""},
		{http.MethodDelete, "/api/v1/runs/999", ""},
	} {
		r := httptest.NewRequest(request.method, request.path, strings.NewReader(request.body))
		r.Header.Set("Authorization", authorization)
		handler.ServeHTTP(httptest.NewRecorder(), r)
	}

	// A closed database makes the next call fail.
	_ = store.Close()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	r.Header.Set("Authorization", authorization)
	handler.ServeHTTP(httptest.NewRecorder(), r)

	body := scrapeMetrics(t, m)

	for _, want := range []string{
		`pacer_history_operations_total{operation="save",result="ok"} 1`,
		`pacer_history_operations_total{operation="list",result="ok"} 1`,
		`pacer_history_operations_total{operation="list",result="error"} 1`,
		// A missing run is an answer from the database, not its failure.
		`pacer_history_operations_total{operation="delete",result="ok"} 1`,
		`pacer_history_operations_total{operation="delete",result="error"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape has no %q", want)
		}
	}
}
