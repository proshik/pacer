package telegram

import (
	"bytes"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"

	"gorun/pkg/card"
)

type sentPhoto struct {
	caption string
	photo   []byte
}

// sent is what a local stand-in for Telegram received, by method.
type sent struct {
	photos  chan sentPhoto
	texts   chan string
	edits   chan string
	answers chan string
}

// newSendingService wires a service whose sender talks to a local stand-in for
// Telegram, so every kind of reply can be read back as it is sent.
func newSendingService(t *testing.T) (*Service, sent) {
	t.Helper()

	out := sent{
		photos:  make(chan sentPhoto, 1),
		texts:   make(chan string, 1),
		edits:   make(chan string, 1),
		answers: make(chan string, 1),
	}
	offer := func(ch chan string, value string) {
		select {
		case ch <- value:
		default:
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		result := `{"message_id":1,"date":0,"chat":{"id":42,"type":"private"}}`
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendPhoto"):
			file, _, err := r.FormFile("photo")
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer func() { _ = file.Close() }()

			data, err := io.ReadAll(file)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			select {
			case out.photos <- sentPhoto{caption: r.FormValue("caption"), photo: data}:
			default:
			}
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			offer(out.texts, r.FormValue("text"))
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			offer(out.edits, r.FormValue("text"))
		case strings.HasSuffix(r.URL.Path, "/answerCallbackQuery"):
			offer(out.answers, r.FormValue("callback_query_id"))
			result = "true"
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":` + result + `}`))
	}))
	t.Cleanup(server.Close)

	client, err := bot.New("test-token", bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatalf("create bot client: %v", err)
	}

	s := newService()
	s.bot = client
	s.startDispatcher()
	s.startSender()
	t.Cleanup(func() {
		close(s.done)
		s.wg.Wait()
	})

	return s, out
}

// /card answers with the plan drawn as a picture, captioned in the sender's
// language so it also reaches someone who cannot see it.
func TestCardCommandSendsThePlanAsAPhoto(t *testing.T) {
	s, out := newSendingService(t)

	s.handleUpdate(withLanguage(newCommandUpdate("/card 21097 1h38m48s"), "ru"))

	select {
	case got := <-out.photos:
		picture, err := png.Decode(bytes.NewReader(got.photo))
		if err != nil {
			t.Fatalf("decode the photo: %v", err)
		}

		if picture.Bounds().Dx() != card.Width || picture.Bounds().Dy() != card.Height {
			t.Errorf("photo is %dx%d, want %dx%d",
				picture.Bounds().Dx(), picture.Bounds().Dy(), card.Width, card.Height)
		}

		if want := "Полумарафон за 1:38:48 — это 4:41 на километр"; got.caption != want {
			t.Errorf("caption = %q, want %q", got.caption, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the card")
	}
}

func TestCardCommandRejectsWrongArguments(t *testing.T) {
	s, out := newSendingService(t)

	s.handleUpdate(newCommandUpdate("/card 21097"))

	select {
	case got := <-out.texts:
		for _, want := range []string{"distance", "time"} {
			if !strings.Contains(got, want) {
				t.Errorf("reply %q does not name %q", got, want)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the reply")
	}
}

// A tap on a button reaches Telegram twice: the answer that stops the
// button's spinner, and the message rewritten in place.
func TestCallbackReachesTelegram(t *testing.T) {
	s, out := newSendingService(t)

	s.handleUpdate(newCallbackUpdate("splits:21097:5928", "ru"))

	for name, ch := range map[string]chan string{"answerCallbackQuery": out.answers, "editMessageText": out.edits} {
		select {
		case got := <-ch:
			if name == "answerCallbackQuery" && got != "query-1" {
				t.Errorf("answered query %q, want query-1", got)
			}
			if name == "editMessageText" && !strings.Contains(got, "финиш") {
				t.Errorf("edited text %q does not show the splits", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %s", name)
		}
	}
}
