package telegram

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"gorun/pkg/metrics"
	"gorun/pkg/plan"
)

func newMetrics(t *testing.T) *metrics.Metrics {
	t.Helper()

	m, err := metrics.New()
	if err != nil {
		t.Fatalf("create metrics: %v", err)
	}

	return m
}

func expectMetrics(t *testing.T, m *metrics.Metrics, name, expected string) {
	t.Helper()

	if err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(expected), name); err != nil {
		t.Fatal(err)
	}
}

func TestHandledUpdatesAreCountedByType(t *testing.T) {
	m := newMetrics(t)
	s := newServiceWithMetrics(m)
	s.startDispatcher()
	t.Cleanup(func() {
		close(s.done)
		s.wg.Wait()
	})

	for _, update := range []*models.Update{
		newCommandUpdate("/time 5m0s 10000"),
		newPlainUpdate("марафон 3:30"),
		newCommandUpdate("/frobnicate"),
		newCallbackUpdate("splits:10000:3000", "en"),
		{InlineQuery: &models.InlineQuery{ID: "inline-1", From: &models.User{}, Query: "10k 50:00"}},
	} {
		s.handleUpdate(update)
		receiveMessage(t, s)
	}

	expectMetrics(t, m, "pacer_telegram_updates_total", `
# HELP pacer_telegram_updates_total Telegram updates handled by the bot, by update type.
# TYPE pacer_telegram_updates_total counter
pacer_telegram_updates_total{type="callback_query"} 1
pacer_telegram_updates_total{type="inline_query"} 1
pacer_telegram_updates_total{type="message"} 3
`)
}

func TestDroppedUpdatesAndRepliesAreCounted(t *testing.T) {
	m := newMetrics(t)
	// No dispatcher and no sender: the queues only fill up.
	s := newServiceWithMetrics(m)

	for range updateQueueSize + 2 {
		s.handleUpdate(newPlainUpdate("10k 50:00"))
	}
	for range messageQueueSize + 1 {
		s.enqueueMessage(textReply{params: &bot.SendMessageParams{ChatID: 42, Text: "x"}})
	}

	expectMetrics(t, m, "pacer_telegram_dropped_total", `
# HELP pacer_telegram_dropped_total Updates and replies dropped because their queue was full.
# TYPE pacer_telegram_dropped_total counter
pacer_telegram_dropped_total{queue="outgoing"} 1
pacer_telegram_dropped_total{queue="updates"} 2
`)
}

// failingReply stands for a reply Telegram refuses.
type failingReply struct{ sent chan struct{} }

func (r failingReply) send(context.Context, *bot.Bot) error {
	defer close(r.sent)

	return errors.New("telegram said no")
}

func TestSendErrorsAreCounted(t *testing.T) {
	m := newMetrics(t)
	s := newServiceWithMetrics(m)
	s.startSender()
	t.Cleanup(func() {
		close(s.done)
		s.wg.Wait()
	})

	reply := failingReply{sent: make(chan struct{})}
	s.enqueueMessage(reply)

	select {
	case <-reply.sent:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the sender")
	}

	// The counter is bumped right after send returns, on the sender goroutine.
	expected := `
# HELP pacer_telegram_send_errors_total Replies Telegram did not accept, by reply kind.
# TYPE pacer_telegram_send_errors_total counter
pacer_telegram_send_errors_total{reply="callback"} 0
pacer_telegram_send_errors_total{reply="greeting"} 0
pacer_telegram_send_errors_total{reply="inline"} 0
pacer_telegram_send_errors_total{reply="other"} 1
pacer_telegram_send_errors_total{reply="photo"} 0
pacer_telegram_send_errors_total{reply="text"} 0
`
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(expected), "pacer_telegram_send_errors_total")
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestReplyKind(t *testing.T) {
	tests := []struct {
		reply outgoing
		want  string
	}{
		{textReply{}, "text"},
		{photoReply{}, "photo"},
		{greetingReply{}, "greeting"},
		{callbackReply{}, "callback"},
		{inlineReply{}, "inline"},
		{failingReply{}, "other"},
	}

	for _, tt := range tests {
		if got := replyKind(tt.reply); got != tt.want {
			t.Errorf("replyKind(%T) = %q, want %q", tt.reply, got, tt.want)
		}
	}
}

// A card that does not draw leaves the user with an apology instead of the
// picture; that counts as an error of the card component.
func TestCardThatDoesNotDrawIsCounted(t *testing.T) {
	m := newMetrics(t)
	s := newServiceWithMetrics(m)

	// Render refuses a plan without distance or time.
	if _, ok := s.cardPhoto(42, plan.Plan{}, &englishTexts); ok {
		t.Fatal("an empty plan drew a card")
	}

	expectMetrics(t, m, "pacer_errors_total", errorsAt(0, 1))
}

// The greeting falls back to words when its picture does not draw; the user
// still gets an answer, but the broken picture is counted.
func TestGreetingPictureThatDoesNotDrawIsCounted(t *testing.T) {
	previous := greetingPicture
	greetingPicture = map[string]func() ([]byte, error){
		"en": func() ([]byte, error) { return nil, errors.New("no fonts") },
	}
	t.Cleanup(func() { greetingPicture = previous })

	telegram := newGreetingTelegram()
	client := telegram.serve(t)
	m := newMetrics(t)
	s := newSiteService(site)
	s.metrics = m

	sendGreeting(t, s, client, "en")

	if len(telegram.texts) != 1 {
		t.Fatalf("the greeting did not fall back to text")
	}
	expectMetrics(t, m, "pacer_errors_total", errorsAt(0, 1))
}

// Sharing that Telegram refuses or does not answer in time is Telegram's
// failure; a page that went away before the answer is not.
func TestRefusedSharingIsCounted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request"}`))
	}))
	t.Cleanup(server.Close)
	client, err := bot.New("test", bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	m := newMetrics(t)
	s := newSiteService(site)
	s.metrics = m
	s.bot = client
	shared := plan.Plan{Distance: 42195, Time: 210 * time.Minute}

	if _, err := s.PrepareInlineMessage(context.Background(), 42, shared, "ru"); err == nil {
		t.Fatal("a refused share succeeded")
	}

	gone, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.PrepareInlineMessage(gone, 42, shared, "ru"); err == nil {
		t.Fatal("a share with a cancelled context succeeded")
	}

	expectMetrics(t, m, "pacer_errors_total", errorsAt(1, 0))
}

func TestHarmlessRefusal(t *testing.T) {
	notModified := fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: message is not modified: specified new message content and reply markup are exactly the same")
	tooOld := fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: query is too old and response timeout expired or query ID is invalid")
	chatNotFound := fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: chat not found")

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "same view tapped twice", err: notModified, want: true},
		{name: "button from before a restart", err: tooOld, want: true},
		{name: "both at once", err: errors.Join(tooOld, notModified), want: true},
		{name: "old button, edit really failed", err: errors.Join(tooOld, chatNotFound), want: false},
		{name: "other bad request", err: chatNotFound, want: false},
		{name: "not a bad request", err: errors.New("message is not modified"), want: false},
		{name: "network", err: context.DeadlineExceeded, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := harmlessRefusal(tt.err); got != tt.want {
				t.Fatalf("harmlessRefusal = %v, want %v", got, tt.want)
			}
		})
	}
}

// replyFailingWith stands for a reply Telegram refuses with err.
type replyFailingWith struct {
	err  error
	sent chan struct{}
}

func (r replyFailingWith) send(context.Context, *bot.Bot) error {
	defer func() { r.sent <- struct{}{} }()

	return r.err
}

// A second tap on the same button is a send error but not a failure: the
// general error counter, which alerts watch, leaves it out.
func TestHarmlessRefusalsStayOutOfTheErrorCounter(t *testing.T) {
	m := newMetrics(t)
	s := newServiceWithMetrics(m)
	s.startSender()
	t.Cleanup(func() {
		close(s.done)
		s.wg.Wait()
	})

	sent := make(chan struct{}, 2)
	s.enqueueMessage(replyFailingWith{err: fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: message is not modified"), sent: sent})
	s.enqueueMessage(replyFailingWith{err: errors.New("telegram said no"), sent: sent})
	for range 2 {
		select {
		case <-sent:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for the sender")
		}
	}

	// Both are send errors; only the real one is a failure. The counters are
	// bumped right after send returns, on the sender goroutine.
	eventuallyMetrics(t, m, "pacer_telegram_send_errors_total", `
# HELP pacer_telegram_send_errors_total Replies Telegram did not accept, by reply kind.
# TYPE pacer_telegram_send_errors_total counter
pacer_telegram_send_errors_total{reply="callback"} 0
pacer_telegram_send_errors_total{reply="greeting"} 0
pacer_telegram_send_errors_total{reply="inline"} 0
pacer_telegram_send_errors_total{reply="other"} 2
pacer_telegram_send_errors_total{reply="photo"} 0
pacer_telegram_send_errors_total{reply="text"} 0
`)
	eventuallyMetrics(t, m, "pacer_errors_total", errorsAt(1, 0))
}

// eventuallyMetrics waits up to two seconds for a metric another goroutine
// records.
func eventuallyMetrics(t *testing.T, m *metrics.Metrics, name, expected string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(expected), name)
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
