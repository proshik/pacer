package metrics

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newMetrics(t *testing.T) *Metrics {
	t.Helper()

	m, err := New()
	if err != nil {
		t.Fatalf("create metrics: %v", err)
	}

	return m
}

func scrape(t *testing.T, handler http.Handler, authorization string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, Path, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder
}

func TestHandlerChecksBearerToken(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		authorization string
		want          int
	}{
		{name: "no header", token: "secret", want: http.StatusUnauthorized},
		{name: "wrong token", token: "secret", authorization: "Bearer wrong", want: http.StatusUnauthorized},
		{name: "token without scheme", token: "secret", authorization: "secret", want: http.StatusUnauthorized},
		{name: "other scheme", token: "secret", authorization: "Basic secret", want: http.StatusUnauthorized},
		{name: "empty token rejects an empty bearer", token: "", authorization: "Bearer ", want: http.StatusUnauthorized},
		{name: "empty token rejects no header", token: "", want: http.StatusUnauthorized},
		{name: "right token", token: "secret", authorization: "Bearer secret", want: http.StatusOK},
	}

	m := newMetrics(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := scrape(t, m.Handler(tt.token), tt.authorization)

			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}
			if tt.want != http.StatusOK && strings.Contains(recorder.Body.String(), "go_goroutines") {
				t.Fatalf("a rejected scrape leaked metrics: %q", recorder.Body.String())
			}
		})
	}
}

func TestScrapeHasRuntimeAndPacerMetrics(t *testing.T) {
	m := newMetrics(t)
	m.ObserveHTTP("/healthz", http.MethodGet, http.StatusOK, 3*time.Millisecond)
	m.BotUpdate(UpdateMessage)
	m.HistoryOp(HistorySave, errors.New("disk full"))

	body := scrape(t, m.Handler("secret"), "Bearer secret").Body.String()

	for _, want := range []string{
		"go_goroutines ",
		"process_",
		`pacer_http_requests_total{code="200",method="GET",route="/healthz"} 1`,
		`pacer_http_request_duration_seconds_count{method="GET",route="/healthz"} 1`,
		`pacer_telegram_updates_total{type="message"} 1`,
		`pacer_telegram_updates_total{type="inline_query"} 0`,
		`pacer_telegram_dropped_total{queue="outgoing"} 0`,
		`pacer_telegram_send_errors_total{reply="photo"} 0`,
		`pacer_history_operations_total{operation="save",result="error"} 1`,
		`pacer_history_operations_total{operation="list",result="ok"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape has no %q", want)
		}
	}
}

// Labels come from fixed sets, whatever a client or a caller sends.
func TestLabelsStayInFixedSets(t *testing.T) {
	m := newMetrics(t)
	m.ObserveHTTP("/", "BREW", http.StatusTeapot, time.Millisecond)
	m.BotSendError("something-new")

	body := scrape(t, m.Handler("secret"), "Bearer secret").Body.String()

	if strings.Contains(body, "BREW") || strings.Contains(body, "something-new") {
		t.Fatalf("an arbitrary value became a label:\n%s", body)
	}
	for _, want := range []string{
		`pacer_http_requests_total{code="418",method="other",route="/"} 1`,
		`pacer_telegram_send_errors_total{reply="other"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape has no %q", want)
		}
	}
}

func TestNilMetricsRecordNothing(t *testing.T) {
	var m *Metrics

	m.ObserveHTTP("/", http.MethodGet, http.StatusOK, time.Millisecond)
	m.BotUpdate(UpdateMessage)
	m.BotSendError("text")
	m.BotDropped(QueueUpdates)
	m.HistoryOp(HistoryList, nil)

	var s *Server
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown of a nil server: %v", err)
	}
}

func TestListenServesScrapeAndShutsDown(t *testing.T) {
	server, err := newMetrics(t).Listen("127.0.0.1:0", "secret")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	get := func(path, authorization string) (int, string) {
		t.Helper()

		request, err := http.NewRequest(http.MethodGet, "http://"+server.Addr()+path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		defer func() { _ = response.Body.Close() }()

		body, _ := io.ReadAll(response.Body)

		return response.StatusCode, string(body)
	}

	if code, _ := get(Path, ""); code != http.StatusUnauthorized {
		t.Errorf("scrape without token: status = %d, want 401", code)
	}
	if code, body := get(Path, "Bearer secret"); code != http.StatusOK || !strings.Contains(body, "go_goroutines") {
		t.Errorf("scrape with token: status = %d, body has go_goroutines = %v", code, strings.Contains(body, "go_goroutines"))
	}
	if code, _ := get("/", "Bearer secret"); code != http.StatusNotFound {
		t.Errorf("other path: status = %d, want 404", code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if response, err := http.Get("http://" + server.Addr() + Path); err == nil {
		_ = response.Body.Close()
		t.Fatal("the listener still answers after shutdown")
	}
}

func TestListenFailsOnBusyAddress(t *testing.T) {
	first, err := newMetrics(t).Listen("127.0.0.1:0", "secret")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = first.Shutdown(context.Background()) })

	if _, err := newMetrics(t).Listen(first.Addr(), "secret"); err == nil {
		t.Fatal("expected an error listening on a busy address")
	}
}
