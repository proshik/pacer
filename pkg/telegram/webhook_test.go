package telegram

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"

	"gorun/pkg/metrics"
)

const (
	webhookTestToken = "123456:secret-bot-token"
	webhookTestURL   = "https://pacer.example/" + webhookTestToken
)

// fakeTelegram answers getWebhookInfo with whatever the test sets and records
// every Bot API method it was asked for.
type fakeTelegram struct {
	mu      sync.Mutex
	info    string // the result object of getWebhookInfo, as JSON
	fail    bool
	methods []string
	checked chan struct{}
	url     string // where the fake listens
}

func newFakeTelegram(t *testing.T, info string) (*fakeTelegram, *bot.Bot) {
	t.Helper()

	fake := &fakeTelegram{info: info, checked: make(chan struct{}, 16)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

		fake.mu.Lock()
		fake.methods = append(fake.methods, method)
		info, fail := fake.info, fake.fail
		fake.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case method == "getUpdates":
			// Long polling: hold the request a little, as Telegram would.
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
		case method != "getWebhookInfo":
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		case fail:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":502,"description":"Bad Gateway"}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":` + info + `}`))
		}

		if method == "getWebhookInfo" {
			select {
			case fake.checked <- struct{}{}:
			default:
			}
		}
	}))
	t.Cleanup(server.Close)
	fake.url = server.URL

	client, err := bot.New(webhookTestToken, bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatalf("create bot client: %v", err)
	}

	return fake, client
}

func (f *fakeTelegram) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.methods...)
}

// webhookService is a service in webhook mode, with metrics, talking to
// client.
func webhookService(t *testing.T, client *bot.Bot) (*Service, *metrics.Metrics) {
	t.Helper()

	m := newMetrics(t)
	s := newServiceWithMetrics(m)
	s.bot = client
	s.webhookURL = webhookTestURL

	return s, m
}

func TestWebhookCheckReportsWhatTelegramSays(t *testing.T) {
	tests := []struct {
		name     string
		info     string
		expected string
	}{
		{
			name: "our webhook, nothing waiting",
			info: `{"url":"` + webhookTestURL + `","pending_update_count":0}`,
			expected: `
# HELP pacer_telegram_webhook_last_error_timestamp_seconds Unix time of the last failed webhook delivery Telegram reports, 0 when none. Webhook mode only.
# TYPE pacer_telegram_webhook_last_error_timestamp_seconds gauge
pacer_telegram_webhook_last_error_timestamp_seconds 0
# HELP pacer_telegram_webhook_ours 1 when Telegram's webhook points at this deployment's URL, 0 otherwise. Webhook mode only.
# TYPE pacer_telegram_webhook_ours gauge
pacer_telegram_webhook_ours 1
# HELP pacer_telegram_webhook_pending_updates Updates Telegram holds and has not delivered yet, as of the last check. Webhook mode only.
# TYPE pacer_telegram_webhook_pending_updates gauge
pacer_telegram_webhook_pending_updates 0
`,
		},
		{
			name: "our webhook failing to deliver",
			info: `{"url":"` + webhookTestURL + `","pending_update_count":12,"last_error_date":1763500000,"last_error_message":"Wrong response from the webhook: 502 Bad Gateway"}`,
			expected: `
# HELP pacer_telegram_webhook_last_error_timestamp_seconds Unix time of the last failed webhook delivery Telegram reports, 0 when none. Webhook mode only.
# TYPE pacer_telegram_webhook_last_error_timestamp_seconds gauge
pacer_telegram_webhook_last_error_timestamp_seconds 1.7635e+09
# HELP pacer_telegram_webhook_ours 1 when Telegram's webhook points at this deployment's URL, 0 otherwise. Webhook mode only.
# TYPE pacer_telegram_webhook_ours gauge
pacer_telegram_webhook_ours 1
# HELP pacer_telegram_webhook_pending_updates Updates Telegram holds and has not delivered yet, as of the last check. Webhook mode only.
# TYPE pacer_telegram_webhook_pending_updates gauge
pacer_telegram_webhook_pending_updates 12
`,
		},
		{
			name: "webhook removed",
			info: `{"url":"","pending_update_count":4}`,
			expected: `
# HELP pacer_telegram_webhook_last_error_timestamp_seconds Unix time of the last failed webhook delivery Telegram reports, 0 when none. Webhook mode only.
# TYPE pacer_telegram_webhook_last_error_timestamp_seconds gauge
pacer_telegram_webhook_last_error_timestamp_seconds 0
# HELP pacer_telegram_webhook_ours 1 when Telegram's webhook points at this deployment's URL, 0 otherwise. Webhook mode only.
# TYPE pacer_telegram_webhook_ours gauge
pacer_telegram_webhook_ours 0
# HELP pacer_telegram_webhook_pending_updates Updates Telegram holds and has not delivered yet, as of the last check. Webhook mode only.
# TYPE pacer_telegram_webhook_pending_updates gauge
pacer_telegram_webhook_pending_updates 4
`,
		},
		{
			name: "webhook of another deployment",
			info: `{"url":"https://other.example/` + webhookTestToken + `","pending_update_count":0}`,
			expected: `
# HELP pacer_telegram_webhook_last_error_timestamp_seconds Unix time of the last failed webhook delivery Telegram reports, 0 when none. Webhook mode only.
# TYPE pacer_telegram_webhook_last_error_timestamp_seconds gauge
pacer_telegram_webhook_last_error_timestamp_seconds 0
# HELP pacer_telegram_webhook_ours 1 when Telegram's webhook points at this deployment's URL, 0 otherwise. Webhook mode only.
# TYPE pacer_telegram_webhook_ours gauge
pacer_telegram_webhook_ours 0
# HELP pacer_telegram_webhook_pending_updates Updates Telegram holds and has not delivered yet, as of the last check. Webhook mode only.
# TYPE pacer_telegram_webhook_pending_updates gauge
pacer_telegram_webhook_pending_updates 0
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := newFakeTelegram(t, tt.info)
			s, m := webhookService(t, client)

			s.checkWebhook(context.Background(), 0)

			expectMetrics(t, m, "pacer_telegram_webhook_ours", tt.expected)
			expectMetrics(t, m, "pacer_telegram_webhook_pending_updates", tt.expected)
			expectMetrics(t, m, "pacer_telegram_webhook_last_error_timestamp_seconds", tt.expected)
			expectMetrics(t, m, "pacer_errors_total", errorsAt(0, 0))
		})
	}
}

func TestFailedWebhookCheckIsCountedWithoutLeakingTheToken(t *testing.T) {
	fake, client := newFakeTelegram(t, `{}`)
	fake.mu.Lock()
	fake.fail = true
	fake.mu.Unlock()
	s, m := webhookService(t, client)

	logs := captureLogs(t)
	s.checkWebhook(context.Background(), 0)

	// The token is the path of a working client too: a server that is not
	// there makes the library's transport error carry the request URL.
	unreachable, err := bot.New(webhookTestToken, bot.WithSkipGetMe(), bot.WithServerURL("http://127.0.0.1:1"))
	if err != nil {
		t.Fatalf("create bot client: %v", err)
	}
	s.bot = unreachable
	s.checkWebhook(context.Background(), 0)

	expectMetrics(t, m, "pacer_telegram_webhook_checks_total", `
# HELP pacer_telegram_webhook_checks_total getWebhookInfo calls by result; error means Telegram did not answer.
# TYPE pacer_telegram_webhook_checks_total counter
pacer_telegram_webhook_checks_total{result="error"} 2
pacer_telegram_webhook_checks_total{result="ok"} 0
`)
	expectMetrics(t, m, "pacer_errors_total", errorsAt(2, 0))

	if strings.Contains(logs.String(), "secret-bot-token") {
		t.Fatalf("the bot token reached the log:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "telegram webhook check failed") {
		t.Fatalf("the failed check was not logged:\n%s", logs.String())
	}
}

// Telegram's webhook URL holds the token; whatever the check logs must not.
func TestWebhookCheckLogsNeitherURLNorToken(t *testing.T) {
	_, client := newFakeTelegram(t, `{"url":"https://other.example/`+webhookTestToken+`","pending_update_count":1,"last_error_date":1763500000,"last_error_message":"Connection timed out"}`)
	s, _ := webhookService(t, client)

	logs := captureLogs(t)
	seen := s.checkWebhook(context.Background(), 0)

	if seen != 1763500000 {
		t.Fatalf("last error seen = %d, want 1763500000", seen)
	}
	out := logs.String()
	if strings.Contains(out, "secret-bot-token") || strings.Contains(out, "other.example") {
		t.Fatalf("the webhook URL reached the log:\n%s", out)
	}
	for _, want := range []string{"telegram webhook points elsewhere", "telegram webhook delivery failed", "Connection timed out"} {
		if !strings.Contains(out, want) {
			t.Errorf("log has no %q:\n%s", want, out)
		}
	}

	// The same delivery error is reported once, not on every check.
	logs.Reset()
	s.checkWebhook(context.Background(), seen)
	if strings.Contains(logs.String(), "telegram webhook delivery failed") {
		t.Fatalf("an old delivery error was logged again:\n%s", logs.String())
	}
}

// The watcher checks right away and then on every tick, and Close stops it
// without touching the webhook: under a start-first rollout the old container
// stops after the new one has set the webhook, so removing it on the way out
// would leave the bot deaf.
func TestWatchWebhookChecksUntilCloseAndLeavesTheWebhook(t *testing.T) {
	fake, client := newFakeTelegram(t, `{"url":"`+webhookTestURL+`","pending_update_count":0}`)
	s, _ := webhookService(t, client)
	s.startDispatcher()
	s.startSender()

	s.WatchWebhook(20 * time.Millisecond)

	for range 3 {
		select {
		case <-fake.checked:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for a webhook check")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}

	checks := len(fake.calls())
	time.Sleep(60 * time.Millisecond)

	calls := fake.calls()
	if len(calls) != checks {
		t.Fatalf("checks went on after Close: %d, then %d", checks, len(calls))
	}
	for _, method := range calls {
		if method != "getWebhookInfo" {
			t.Fatalf("the service called %s; it must only read the webhook", method)
		}
	}
}

// Polling mode, a service without metrics and a non-positive interval start
// no watcher at all.
func TestWatchWebhookIsOffOutsideWebhookMode(t *testing.T) {
	tests := []struct {
		name     string
		polling  bool
		metrics  bool
		interval time.Duration
	}{
		{name: "polling", polling: true, metrics: true, interval: time.Millisecond},
		{name: "no metrics", metrics: false, interval: time.Millisecond},
		{name: "zero interval", metrics: true, interval: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, client := newFakeTelegram(t, `{"url":""}`)
			s := newService()
			if tt.metrics {
				s = newServiceWithMetrics(newMetrics(t))
			}
			s.bot = client
			s.webhookURL = webhookTestURL
			s.polling = tt.polling

			s.WatchWebhook(tt.interval)
			time.Sleep(30 * time.Millisecond)
			if err := s.Close(context.Background()); err != nil {
				t.Fatalf("close: %v", err)
			}

			if calls := fake.calls(); len(calls) != 0 {
				t.Fatalf("unexpected Telegram calls: %v", calls)
			}
		})
	}
}

// NewService end to end against a fake Telegram: in webhook mode it sets the
// webhook, the watcher sees it as ours, and Close stops everything without
// removing it. In debug mode the webhook is removed at startup, as before, and
// there is no watcher.
func TestNewServiceKeepsTheWebhookOnClose(t *testing.T) {
	tests := []struct {
		name       string
		debug      bool
		wantCalled []string
		notCalled  string
		mode       string
	}{
		{name: "webhook", wantCalled: []string{"setWebhook", "getWebhookInfo"}, notCalled: "deleteWebhook", mode: "webhook"},
		{name: "polling", debug: true, wantCalled: []string{"deleteWebhook", "getUpdates"}, notCalled: "getWebhookInfo", mode: "polling"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, _ := newFakeTelegram(t, `{"url":"https://PACER.example:443/`+webhookTestToken+`","pending_update_count":0}`)
			previous := testClientOptions
			testClientOptions = []bot.Option{bot.WithSkipGetMe(), bot.WithServerURL(fake.url)}
			t.Cleanup(func() { testClientOptions = previous })

			m := newMetrics(t)
			s, err := NewService(context.Background(), tt.debug, "pacer.example", webhookTestToken, m)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			s.WatchWebhook(time.Hour)

			if !tt.debug {
				select {
				case <-fake.checked:
				case <-time.After(2 * time.Second):
					t.Fatal("timed out waiting for the first webhook check")
				}
				// The gauge is set right after the answer arrives.
				eventuallyMetrics(t, m, "pacer_telegram_webhook_ours", `
# HELP pacer_telegram_webhook_ours 1 when Telegram's webhook points at this deployment's URL, 0 otherwise. Webhook mode only.
# TYPE pacer_telegram_webhook_ours gauge
pacer_telegram_webhook_ours 1
`)
			} else {
				time.Sleep(60 * time.Millisecond)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := s.Close(ctx); err != nil {
				t.Fatalf("close: %v", err)
			}

			calls := fake.calls()
			for _, want := range tt.wantCalled {
				if !slices.Contains(calls, want) {
					t.Errorf("%s was not called: %v", want, calls)
				}
			}
			if slices.Contains(calls, tt.notCalled) {
				t.Errorf("%s was called: %v", tt.notCalled, calls)
			}
			expectMetrics(t, m, "pacer_telegram_mode", `
# HELP pacer_telegram_mode How the bot gets updates: 1 for the active mode, 0 for the other.
# TYPE pacer_telegram_mode gauge
pacer_telegram_mode{mode="polling"} `+map[bool]string{true: "1", false: "0"}[tt.debug]+`
pacer_telegram_mode{mode="webhook"} `+map[bool]string{true: "0", false: "1"}[tt.debug]+`
`)
		})
	}
}

func TestSameWebhookURL(t *testing.T) {
	ours := "https://pacer.example/" + webhookTestToken
	tests := []struct {
		reported string
		want     bool
	}{
		{reported: ours, want: true},
		{reported: "https://PACER.Example/" + webhookTestToken, want: true},
		{reported: "https://pacer.example:443/" + webhookTestToken, want: true},
		{reported: "HTTPS://pacer.example/" + webhookTestToken, want: true},
		{reported: "", want: false},
		{reported: "https://pacer.example:8443/" + webhookTestToken, want: false},
		{reported: "https://other.example/" + webhookTestToken, want: false},
		{reported: "https://pacer.example/" + webhookTestToken + "/", want: false},
		{reported: "https://pacer.example/654321:other-token", want: false},
		{reported: "http://pacer.example/" + webhookTestToken, want: false},
	}

	for _, tt := range tests {
		if got := sameWebhookURL(tt.reported, ours); got != tt.want {
			t.Errorf("sameWebhookURL(%q) = %v, want %v", strings.ReplaceAll(tt.reported, webhookTestToken, "<token>"), got, tt.want)
		}
	}
}

// errorsAt is the expected pacer_errors_total with the given telegram and card
// counts and nothing else.
func errorsAt(telegram, card int) string {
	return `
# HELP pacer_errors_total Failures a user felt or an outside service caused, by component.
# TYPE pacer_errors_total counter
pacer_errors_total{component="calc"} 0
pacer_errors_total{component="card"} ` + strconv.Itoa(card) + `
pacer_errors_total{component="history"} 0
pacer_errors_total{component="queue"} 0
pacer_errors_total{component="telegram"} ` + strconv.Itoa(telegram) + `
`
}

// captureLogs routes the default logger into a buffer for the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &buf
}
