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

	"gorun/pkg/calculator"
	"gorun/pkg/card"
)

type sentPhoto struct {
	caption string
	photo   []byte
}

// newSendingService wires a service whose sender talks to a local stand-in for
// Telegram, so both a photo and a text reply can be read back as they are sent.
func newSendingService(t *testing.T) (*Service, <-chan sentPhoto, <-chan string) {
	t.Helper()

	photos := make(chan sentPhoto, 1)
	texts := make(chan string, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

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
			case photos <- sentPhoto{caption: r.FormValue("caption"), photo: data}:
			default:
			}
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			select {
			case texts <- r.FormValue("text"):
			default:
			}
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":42,"type":"private"}}}`))
	}))
	t.Cleanup(server.Close)

	client, err := bot.New("test-token", bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
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

	return s, photos, texts
}

// /card answers with the plan drawn as a picture, captioned in the sender's
// language so it also reaches someone who cannot see it.
func TestCardCommandSendsThePlanAsAPhoto(t *testing.T) {
	s, photos, _ := newSendingService(t)

	s.handleUpdate(withLanguage(newCommandUpdate("/card 21097 1h38m48s"), "ru"))

	select {
	case got := <-photos:
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
	s, _, texts := newSendingService(t)

	s.handleUpdate(newCommandUpdate("/card 21097"))

	select {
	case got := <-texts:
		for _, want := range []string{"distance", "time"} {
			if !strings.Contains(got, want) {
				t.Errorf("reply %q does not name %q", got, want)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the reply")
	}
}
