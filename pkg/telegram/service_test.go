package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"gorun/pkg/calculator"
)

// newCommandUpdate builds an update shaped the way Telegram sends a command:
// a bot_command entity covering the leading "/cmd" token.
func newCommandUpdate(text string) *models.Update {
	command := text
	if i := strings.Index(text, " "); i != -1 {
		command = text[:i]
	}

	return &models.Update{
		Message: &models.Message{
			Text: text,
			Entities: []models.MessageEntity{{
				Type:   models.MessageEntityTypeBotCommand,
				Offset: 0,
				Length: len(command),
			}},
			Chat: models.Chat{ID: 42},
		},
	}
}

func newPlainUpdate(text string) *models.Update {
	return &models.Update{
		Message: &models.Message{Text: text, Chat: models.Chat{ID: 42}},
	}
}

func messageText(t *testing.T, params *bot.SendMessageParams) string {
	t.Helper()

	if params == nil {
		t.Fatal("expected outgoing message, got nil")
	}

	return params.Text
}

// newTestService wires channels and the dispatcher without a bot client;
// tests read outgoing messages straight off s.messages.
func newTestService(t *testing.T) *Service {
	t.Helper()

	s := newService(calculator.NewService())
	s.startDispatcher()

	t.Cleanup(func() {
		close(s.done)
		s.wg.Wait()
	})

	return s
}

func receiveMessage(t *testing.T, s *Service) *bot.SendMessageParams {
	t.Helper()

	select {
	case msg := <-s.messages:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for an outgoing message")
		return nil
	}
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name          string
		update        *models.Update
		wantCommand   string
		wantArguments string
	}{
		{
			name:          "command with arguments",
			update:        newCommandUpdate("/time 5m0s 10000"),
			wantCommand:   "time",
			wantArguments: "5m0s 10000",
		},
		{
			name:        "bare command",
			update:      newCommandUpdate("/start"),
			wantCommand: "start",
		},
		{
			name:          "command addressed to a specific bot",
			update:        newCommandUpdate("/pace@pacer_bot 21097 1h38m48s"),
			wantCommand:   "pace",
			wantArguments: "21097 1h38m48s",
		},
		{
			name:   "plain message is not a command",
			update: newPlainUpdate("привет"),
		},
		{
			name: "entity away from the start is not a command",
			update: &models.Update{Message: &models.Message{
				Text: "look at /time",
				Entities: []models.MessageEntity{{
					Type:   models.MessageEntityTypeBotCommand,
					Offset: 8,
					Length: 5,
				}},
				Chat: models.Chat{ID: 42},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, arguments := parseCommand(tt.update.Message)

			if command != tt.wantCommand {
				t.Errorf("command = %q, want %q", command, tt.wantCommand)
			}
			if arguments != tt.wantArguments {
				t.Errorf("arguments = %q, want %q", arguments, tt.wantArguments)
			}
		})
	}
}

func TestHandleTimeCmdReportsOffendingPaceArgument(t *testing.T) {
	s := newService(calculator.NewService())

	text := messageText(t, s.handleTimeCmd(newCommandUpdate("/time notapace 21095")))

	if !strings.Contains(text, "notapace") {
		t.Fatalf("expected the error to quote the pace argument, got %q", text)
	}
}

func TestHandlePaceCmdReportsOffendingTimeArgument(t *testing.T) {
	s := newService(calculator.NewService())

	text := messageText(t, s.handlePaceCmd(newCommandUpdate("/pace 21097 nottime")))

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

	s.handleUpdate(newPlainUpdate("привет"))

	text := messageText(t, receiveMessage(t, s))
	if !strings.Contains(text, "/time") || !strings.Contains(text, "/pace") {
		t.Fatalf("expected the greeting to list both commands, got %q", text)
	}
}

func TestHandleTimeCmdCalculatesTime(t *testing.T) {
	s := newService(calculator.NewService())

	if got, want := messageText(t, s.handleTimeCmd(newCommandUpdate("/time 5m0s 10000"))), "50m0s"; got != want {
		t.Fatalf("time for 10 km at 5:00/km = %q, want %q", got, want)
	}
}

func TestHandlePaceCmdCalculatesPace(t *testing.T) {
	s := newService(calculator.NewService())

	if got, want := messageText(t, s.handlePaceCmd(newCommandUpdate("/pace 10000 50m0s"))), "5m0s"; got != want {
		t.Fatalf("pace for 10 km in 50:00 = %q, want %q", got, want)
	}
}

func TestCommandsRejectWrongArgumentCount(t *testing.T) {
	s := newService(calculator.NewService())

	for _, text := range []string{"/time 5m0s", "/pace 10000"} {
		var got string
		if strings.HasPrefix(text, "/time") {
			got = messageText(t, s.handleTimeCmd(newCommandUpdate(text)))
		} else {
			got = messageText(t, s.handlePaceCmd(newCommandUpdate(text)))
		}

		if !strings.Contains(got, "2 arguments") {
			t.Errorf("%q: expected a complaint about argument count, got %q", text, got)
		}
	}
}

func TestUpdateWithoutMessageIsIgnored(t *testing.T) {
	s := newTestService(t)

	s.handleUpdate(&models.Update{})

	select {
	case msg := <-s.messages:
		t.Fatalf("expected no reply, got %q", msg.Text)
	case <-time.After(200 * time.Millisecond):
	}
}

// The sender goroutine is the only place that talks to Telegram. Pointing the
// client at a local server proves an enqueued message really is delivered as a
// sendMessage call, which nothing covered before.
func TestSenderDeliversEnqueuedMessage(t *testing.T) {
	type sentMessage struct {
		ChatID int64
		Text   string
	}

	received := make(chan sentMessage, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// the client encodes calls as multipart/form-data, not JSON
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		chatID, err := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		params := sentMessage{ChatID: chatID, Text: r.FormValue("text")}

		select {
		case received <- params:
		default:
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":42,"type":"private"}}}`))
	}))
	defer server.Close()

	client, err := bot.New("test-token",
		bot.WithSkipGetMe(),
		bot.WithServerURL(server.URL),
	)
	if err != nil {
		t.Fatalf("create bot client: %v", err)
	}

	s := newService(calculator.NewService())
	s.bot = client
	s.startDispatcher()
	s.startSender()
	t.Cleanup(func() {
		close(s.done)
		s.wg.Wait()
	})

	s.handleUpdate(newCommandUpdate("/time 5m0s 10000"))

	select {
	case got := <-received:
		if got.Text != "50m0s" {
			t.Errorf("delivered text = %q, want %q", got.Text, "50m0s")
		}
		if got.ChatID != 42 {
			t.Errorf("delivered chat_id = %d, want 42", got.ChatID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the message to reach Telegram")
	}
}

func TestBuildWebhookURL(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		want      string
		wantError bool
	}{
		{name: "bare host gets https", host: "pacer.example.com", want: "https://pacer.example.com/secret"},
		{name: "https host is kept", host: "https://pacer.example.com", want: "https://pacer.example.com/secret"},
		{name: "trailing slash is trimmed", host: "https://pacer.example.com/", want: "https://pacer.example.com/secret"},
		{name: "sub path is preserved", host: "https://example.com/bots", want: "https://example.com/bots/secret"},
		{name: "query and fragment are dropped", host: "https://example.com/?a=1#x", want: "https://example.com/secret"},
		{name: "plain http is rejected", host: "http://example.com", wantError: true},
		{name: "empty host is rejected", host: "   ", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildWebhookURL(tt.host, "secret")

			if tt.wantError {
				if err == nil {
					t.Fatalf("expected an error for %q, got %q", tt.host, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.host, err)
			}
			if got != tt.want {
				t.Errorf("buildWebhookURL(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	s := newService(calculator.NewService())
	s.startDispatcher()

	ctx := context.Background()
	if err := s.Close(ctx); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := s.Close(ctx); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
