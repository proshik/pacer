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
	m.Error(ComponentCard)
	m.TelegramMode(true)
	m.WebhookChecked(true, 1, time.Now())
	m.WebhookCheckFailed()

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

// The error counter is the one place an alert can watch: failures counted by
// their own metrics also count there, by component, and each component starts
// at zero so rate() sees the series before the first failure.
func TestErrorsAreCountedByComponent(t *testing.T) {
	m := newMetrics(t)

	m.BotSendError("text") // the sender counts it as an error, not this
	m.Error(ComponentTelegram)
	m.BotDropped(QueueUpdates)
	m.BotDropped(QueueOutgoing)
	m.HistoryOp(HistorySave, errors.New("disk full"))
	m.HistoryOp(HistoryList, nil)
	m.WebhookCheckFailed()
	m.Error(ComponentCard)
	m.Error("not-a-component")

	body := scrape(t, m.Handler("secret"), "Bearer secret").Body.String()

	if strings.Contains(body, "not-a-component") {
		t.Fatalf("an arbitrary component became a label:\n%s", body)
	}
	for _, want := range []string{
		`pacer_errors_total{component="telegram"} 2`,
		`pacer_errors_total{component="queue"} 2`,
		`pacer_errors_total{component="history"} 1`,
		`pacer_errors_total{component="card"} 1`,
		`pacer_errors_total{component="calc"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape has no %q", want)
		}
	}
}

// Before the first check there is no webhook series at all: polling mode and
// a check that never succeeded must not read as a webhook that is fine.
func TestWebhookGaugesAppearWithTheFirstCheck(t *testing.T) {
	m := newMetrics(t)
	m.TelegramMode(false)

	body := scrape(t, m.Handler("secret"), "Bearer secret").Body.String()
	for _, absent := range []string{"pacer_telegram_webhook_ours", "pacer_telegram_webhook_pending_updates", "pacer_telegram_webhook_last_error_timestamp_seconds"} {
		if strings.Contains(body, absent+" ") {
			t.Errorf("%s is exported before any check", absent)
		}
	}
	for _, want := range []string{
		`pacer_telegram_mode{mode="webhook"} 1`,
		`pacer_telegram_mode{mode="polling"} 0`,
		`pacer_telegram_webhook_checks_total{result="ok"} 0`,
		`pacer_telegram_webhook_checks_total{result="error"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape has no %q", want)
		}
	}

	lastError := time.Unix(1763500000, 0)
	m.WebhookChecked(true, 3, lastError)
	m.WebhookChecked(false, 0, time.Time{})
	m.WebhookCheckFailed()

	body = scrape(t, m.Handler("secret"), "Bearer secret").Body.String()
	for _, want := range []string{
		"pacer_telegram_webhook_ours 0",
		"pacer_telegram_webhook_pending_updates 0",
		"pacer_telegram_webhook_last_error_timestamp_seconds 0",
		`pacer_telegram_webhook_checks_total{result="ok"} 2`,
		`pacer_telegram_webhook_checks_total{result="error"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape has no %q", want)
		}
	}

	m.WebhookChecked(true, 7, lastError)
	body = scrape(t, m.Handler("secret"), "Bearer secret").Body.String()
	for _, want := range []string{
		"pacer_telegram_webhook_ours 1",
		"pacer_telegram_webhook_pending_updates 7",
		"pacer_telegram_webhook_last_error_timestamp_seconds 1.7635e+09",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape has no %q", want)
		}
	}
}

func TestTelegramModeMarksPolling(t *testing.T) {
	m := newMetrics(t)
	m.TelegramMode(true)

	body := scrape(t, m.Handler("secret"), "Bearer secret").Body.String()
	for _, want := range []string{`pacer_telegram_mode{mode="polling"} 1`, `pacer_telegram_mode{mode="webhook"} 0`} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape has no %q", want)
		}
	}
}
