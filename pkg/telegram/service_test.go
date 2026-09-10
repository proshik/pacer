package telegram

import (
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"gorun/pkg/calculator"
)

// newCommandUpdate builds an update that tgbotapi recognizes as a bot command,
// mirroring how Telegram marks the leading "/cmd" token with an entity.
func newCommandUpdate(text string) tgbotapi.Update {
	command := text
	if i := strings.Index(text, " "); i != -1 {
		command = text[:i]
	}

	entities := []tgbotapi.MessageEntity{{
		Type:   "bot_command",
		Offset: 0,
		Length: len(command),
	}}

	return tgbotapi.Update{
		Message: &tgbotapi.Message{
			Text:     text,
			Entities: &entities,
			Chat:     &tgbotapi.Chat{ID: 42},
		},
	}
}

func messageText(t *testing.T, chattable tgbotapi.Chattable) string {
	t.Helper()

	msg, ok := chattable.(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected tgbotapi.MessageConfig, got %T", chattable)
	}

	return msg.Text
}

// newTestService wires a Service with live channels and a running dispatcher,
// but no bot client: tests read outgoing messages straight off s.messages
// instead of letting the sender goroutine hit the Telegram API.
func newTestService(t *testing.T) *Service {
	t.Helper()

	s := &Service{
		calculator: calculator.NewService(),
		startC:     make(chan tgbotapi.Update, 8),
		timeC:      make(chan tgbotapi.Update, 8),
		paceC:      make(chan tgbotapi.Update, 8),
		unknownC:   make(chan tgbotapi.Update, 8),
		messages:   make(chan tgbotapi.Chattable, 8),
		done:       make(chan struct{}),
	}

	s.startDispatcher()

	t.Cleanup(func() {
		close(s.done)
		s.wg.Wait()
	})

	return s
}

func receiveMessage(t *testing.T, s *Service) tgbotapi.Chattable {
	t.Helper()

	select {
	case msg := <-s.messages:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for an outgoing message")
		return nil
	}
}

func TestHandleTimeCmdReportsOffendingPaceArgument(t *testing.T) {
	s := &Service{calculator: calculator.NewService()}
	update := newCommandUpdate("/time notapace 21095")

	text := messageText(t, s.handleTimeCmd(&update))

	if !strings.Contains(text, "notapace") {
		t.Fatalf("expected the error to quote the pace argument, got %q", text)
	}
}

func TestHandlePaceCmdReportsOffendingTimeArgument(t *testing.T) {
	s := &Service{calculator: calculator.NewService()}
	update := newCommandUpdate("/pace 21097 nottime")

	text := messageText(t, s.handlePaceCmd(&update))

	if !strings.Contains(text, "nottime") {
		t.Fatalf("expected the error to quote the time argument, got %q", text)
	}
}

func TestUnknownCommandIsNotAnsweredWithGreeting(t *testing.T) {
	s := newTestService(t)

	s.handleUpdate(newCommandUpdate("/frobnicate"))

	text := messageText(t, receiveMessage(t, s))
	if !strings.Contains(text, "frobnicate") {
		t.Fatalf("expected the reply to name the unknown command, got %q", text)
	}
}

func TestPlainMessageIsAnsweredWithGreeting(t *testing.T) {
	s := newTestService(t)

	s.handleUpdate(tgbotapi.Update{Message: &tgbotapi.Message{
		Text: "привет",
		Chat: &tgbotapi.Chat{ID: 42},
	}})

	text := messageText(t, receiveMessage(t, s))
	if !strings.Contains(text, "/time") || !strings.Contains(text, "/pace") {
		t.Fatalf("expected the greeting to list both commands, got %q", text)
	}
}

func TestHandleTimeCmdCalculatesTime(t *testing.T) {
	s := &Service{calculator: calculator.NewService()}
	update := newCommandUpdate("/time 5m0s 10000")

	if got, want := messageText(t, s.handleTimeCmd(&update)), "50m0s"; got != want {
		t.Fatalf("time for 10 km at 5:00/km = %q, want %q", got, want)
	}
}

func TestHandlePaceCmdCalculatesPace(t *testing.T) {
	s := &Service{calculator: calculator.NewService()}
	update := newCommandUpdate("/pace 10000 50m0s")

	if got, want := messageText(t, s.handlePaceCmd(&update)), "5m0s"; got != want {
		t.Fatalf("pace for 10 km in 50:00 = %q, want %q", got, want)
	}
}

func TestCommandsRejectWrongArgumentCount(t *testing.T) {
	s := &Service{calculator: calculator.NewService()}

	for _, text := range []string{"/time 5m0s", "/pace 10000"} {
		update := newCommandUpdate(text)

		var got string
		if strings.HasPrefix(text, "/time") {
			got = messageText(t, s.handleTimeCmd(&update))
		} else {
			got = messageText(t, s.handlePaceCmd(&update))
		}

		if !strings.Contains(got, "2 arguments") {
			t.Errorf("%q: expected a complaint about argument count, got %q", text, got)
		}
	}
}
