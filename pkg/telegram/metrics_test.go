package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/prometheus/client_golang/prometheus/testutil"

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
