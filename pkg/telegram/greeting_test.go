package telegram

import (
	"bytes"
	"context"
	"html"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Telegram's limits, from the Bot API: a photo caption holds 1024 characters
// after the markup is parsed, a description 512 and a short description 120.
// Telegram counts UTF-16 units, which is never fewer than the characters.
const (
	captionLimit          = 1024
	descriptionLimit      = 512
	shortDescriptionLimit = 120
)

func utf16Length(text string) int {
	return len(utf16.Encode([]rune(text)))
}

var (
	telegramTag    = regexp.MustCompile(`</?([a-z]+)>`)
	telegramEntity = regexp.MustCompile(`&(amp|lt|gt|quot);`)
)

// visibleHTML checks markup the way Telegram's HTML mode reads it, for the
// tags the greeting uses, and returns the text the reader sees.
func visibleHTML(t *testing.T, markup string) string {
	t.Helper()

	var open []string
	for _, match := range telegramTag.FindAllStringSubmatch(markup, -1) {
		tag, name := match[0], match[1]
		if name != "b" && name != "code" {
			t.Errorf("tag %s is not one the greeting should use: %q", tag, markup)
		}
		if !strings.HasPrefix(tag, "</") {
			open = append(open, name)
			continue
		}
		if len(open) == 0 || open[len(open)-1] != name {
			t.Errorf("%s closes nothing open in %q", tag, markup)
			continue
		}
		open = open[:len(open)-1]
	}
	if len(open) > 0 {
		t.Errorf("tags left open %v in %q", open, markup)
	}

	text := telegramTag.ReplaceAllString(markup, "")
	if strings.ContainsAny(text, "<>") {
		t.Errorf("unescaped < or > in %q", markup)
	}
	if bare := telegramEntity.ReplaceAllString(text, ""); strings.Contains(bare, "&") {
		t.Errorf("unescaped & in %q", markup)
	}

	return html.UnescapeString(text)
}

// The greeting goes out in Telegram's HTML: only tags Telegram knows, closed
// in order, the sender's name escaped, and short enough to be a caption.
func TestGreetingIsTelegramHTML(t *testing.T) {
	names := []string{"", "Анна", `<b>Tom & "Jerry"</b>`}

	for _, words := range []*texts{&russianTexts, &englishTexts} {
		for _, name := range names {
			markup := words.greeting(name)
			visible := visibleHTML(t, markup)

			if n := utf16Length(visible); n > captionLimit {
				t.Errorf("%s greeting for %q is %d characters, over the caption limit", words.language, name, n)
			}
			if name != "" && !strings.Contains(visible, name) {
				t.Errorf("%s greeting does not greet %q: %q", words.language, name, visible)
			}
			if !strings.HasPrefix(markup, "<b>Pacer") {
				t.Errorf("%s greeting does not open with the bold title: %q", words.language, markup)
			}
			for _, e := range append(words.greetingExample, words.greetingCommand...) {
				if !strings.Contains(markup, "<code>"+e.text+"</code>") {
					t.Errorf("%s greeting lacks %q as code: %q", words.language, e.text, markup)
				}
			}
		}
	}
}

// Every example in the greeting has to work when copied into the chat: the
// plain ones read as a plan, the commands answer with one.
func TestGreetingExamplesWork(t *testing.T) {
	s := newService()

	for _, words := range []*texts{&russianTexts, &englishTexts} {
		for _, e := range words.greetingExample {
			reply := s.handleText(withLanguage(newPlainUpdate(e.text), words.language))
			if text := messageText(t, reply); strings.Contains(text, "Pacer") {
				t.Errorf("example %q is answered with the greeting, not a plan", e.text)
			}
		}

		for _, e := range words.greetingCommand {
			update := withLanguage(newCommandUpdate(e.text), words.language)
			var reply outgoing
			switch {
			case strings.HasPrefix(e.text, "/pace"):
				reply = s.handlePaceCmd(update)
			case strings.HasPrefix(e.text, "/time"):
				reply = s.handleTimeCmd(update)
			case strings.HasPrefix(e.text, "/card"):
				reply = s.handleCardCmd(update)
			default:
				t.Fatalf("no handler for example %q", e.text)
			}
			if _, isPhoto := reply.(photoReply); !isPhoto && !strings.Contains(messageText(t, reply), "<b>") {
				t.Errorf("command example %q is not answered with a plan", e.text)
			}
		}
	}
}

// The owner's rule for the bot: the page's quiet style, no emoji anywhere.
func TestBotTextsHaveNoEmoji(t *testing.T) {
	for _, words := range []*texts{&russianTexts, &englishTexts} {
		for _, text := range stringsIn(reflect.ValueOf(*words)) {
			for _, r := range text {
				if isEmoji(r) {
					t.Errorf("%s text %q carries the emoji %U", words.language, text, r)
				}
			}
		}
	}
}

// stringsIn collects every string in a value, including those in slices of
// structs such as the greeting's examples.
func stringsIn(value reflect.Value) []string {
	switch value.Kind() {
	case reflect.String:
		return []string{value.String()}
	case reflect.Struct:
		var out []string
		for i := range value.NumField() {
			out = append(out, stringsIn(value.Field(i))...)
		}
		return out
	case reflect.Slice:
		var out []string
		for i := range value.Len() {
			out = append(out, stringsIn(value.Index(i))...)
		}
		return out
	}

	return nil
}

// isEmoji covers the pictographic blocks and the joiners and selector that
// turn ordinary symbols into emoji. Arrows such as ← in "← Plan" stay text.
func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF, // pictographs, emoticons, transport, symbols
		r >= 0x2600 && r <= 0x27BF, // miscellaneous symbols and dingbats
		r >= 0x2B00 && r <= 0x2BFF, // stars, squares and circles
		r >= 0x2300 && r <= 0x23FF, // watches, hourglasses, media buttons
		r == 0x200D, r == 0xFE0F, r == 0x20E3:
		return true
	}

	return false
}

func TestDescriptionsFitTelegramLimits(t *testing.T) {
	for _, words := range []*texts{&russianTexts, &englishTexts} {
		if n := utf16Length(words.description); n == 0 || n > descriptionLimit {
			t.Errorf("%s description is %d characters, want 1..%d", words.language, n, descriptionLimit)
		}
		if n := utf16Length(words.shortDescription); n == 0 || n > shortDescriptionLimit {
			t.Errorf("%s short description is %d characters, want 1..%d", words.language, n, shortDescriptionLimit)
		}
		if hasCyrillic(words.description+words.shortDescription) != (words.language == "ru") {
			t.Errorf("%s descriptions are in the wrong language", words.language)
		}
	}
}

// The descriptions are published like the menu: one set without a language,
// which everyone but a Russian speaker sees, and one for "ru".
func TestDescriptionsArePublishedInBothLanguages(t *testing.T) {
	type published struct{ method, language, text string }
	received := make(chan published, 8)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		text := r.FormValue("description") + r.FormValue("short_description")
		received <- published{method: method, language: r.FormValue("language_code"), text: text}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client, err := bot.New("test-token", bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatalf("create bot client: %v", err)
	}

	if err := publishDescriptions(context.Background(), client); err != nil {
		t.Fatalf("publish descriptions: %v", err)
	}
	close(received)

	got := map[string]string{}
	for p := range received {
		got[p.method+" "+p.language] = p.text
	}

	want := map[string]string{
		"setMyDescription ":        englishTexts.description,
		"setMyShortDescription ":   englishTexts.shortDescription,
		"setMyDescription ru":      russianTexts.description,
		"setMyShortDescription ru": russianTexts.shortDescription,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("published %v, want %v", got, want)
	}
}

// greetingTelegram is a stand-in for Telegram that records how the greeting
// arrived and fails the ways the test asks it to.
type greetingTelegram struct {
	photos chan string // "upload" or the file_id that was sent
	texts  chan *http.Request
	failID string // a file_id Telegram no longer knows
	failUp bool   // every upload fails
}

func (g *greetingTelegram) serve(t *testing.T) *bot.Bot {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fail := func() {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: wrong file"}`))
		}

		switch {
		case strings.HasSuffix(r.URL.Path, "/sendPhoto"):
			if r.FormValue("parse_mode") != "HTML" || !strings.HasPrefix(r.FormValue("caption"), "<b>Pacer") {
				t.Errorf("greeting photo caption = %q in mode %q", r.FormValue("caption"), r.FormValue("parse_mode"))
			}

			if id := r.FormValue("photo"); id != "" {
				g.photos <- id
				if id == g.failID {
					fail()
					return
				}
			} else {
				file, _, err := r.FormFile("photo")
				if err != nil {
					t.Errorf("sendPhoto without a photo: %v", err)
					fail()
					return
				}
				data, _ := io.ReadAll(file)
				_ = file.Close()
				if _, err := png.Decode(bytes.NewReader(data)); err != nil {
					t.Errorf("uploaded greeting is not a PNG: %v", err)
				}
				g.photos <- "upload"
				if g.failUp {
					fail()
					return
				}
			}

			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":42,"type":"private"},` +
				`"photo":[{"file_id":"small","file_unique_id":"s","width":320,"height":168},` +
				`{"file_id":"large","file_unique_id":"l","width":1200,"height":630}]}}`))
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			g.texts <- r
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":42,"type":"private"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	client, err := bot.New("test-token", bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatalf("create bot client: %v", err)
	}

	return client
}

func newGreetingTelegram() *greetingTelegram {
	return &greetingTelegram{photos: make(chan string, 8), texts: make(chan *http.Request, 8)}
}

func sendGreeting(t *testing.T, s *Service, client *bot.Bot, language string) {
	t.Helper()

	reply := s.handleStartCmd(withLanguage(newCommandUpdate("/start"), language))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := reply.send(ctx, client); err != nil {
		t.Fatalf("send the greeting: %v", err)
	}
}

func next(t *testing.T, ch chan string) string {
	t.Helper()

	select {
	case got := <-ch:
		return got
	default:
		t.Fatal("Telegram received no photo")
		return ""
	}
}

// /start says hello with the picture; the first time per language it is
// uploaded, after that the file_id Telegram gave the largest size is reused.
func TestStartSendsTheGreetingPictureOnce(t *testing.T) {
	telegram := newGreetingTelegram()
	client := telegram.serve(t)
	s := newSiteService(site)

	sendGreeting(t, s, client, "ru")
	sendGreeting(t, s, client, "ru")
	sendGreeting(t, s, client, "en")

	for i, want := range []string{"upload", "large", "upload"} {
		if got := next(t, telegram.photos); got != want {
			t.Errorf("greeting %d sent its photo as %q, want %q", i+1, got, want)
		}
	}
	if len(telegram.texts) != 0 {
		t.Errorf("%d greetings fell back to text with the photo working", len(telegram.texts))
	}
}

// A file_id Telegram rejects is dropped and the picture uploaded again.
func TestStaleGreetingPictureIsUploadedAgain(t *testing.T) {
	telegram := newGreetingTelegram()
	telegram.failID = "gone"
	client := telegram.serve(t)
	s := newSiteService(site)
	s.greetingPhotos.set("ru", "gone")

	sendGreeting(t, s, client, "ru")

	for i, want := range []string{"gone", "upload"} {
		if got := next(t, telegram.photos); got != want {
			t.Errorf("attempt %d sent %q, want %q", i+1, got, want)
		}
	}
	if got := s.greetingPhotos.get("ru"); got != "large" {
		t.Errorf("cached file_id = %q, want the new one", got)
	}
}

// When the photo cannot be sent the bot still answers: the same greeting as a
// message, with the same button.
func TestGreetingFallsBackToText(t *testing.T) {
	telegram := newGreetingTelegram()
	telegram.failUp = true
	client := telegram.serve(t)
	s := newSiteService(site)

	sendGreeting(t, s, client, "en")

	select {
	case r := <-telegram.texts:
		if r.FormValue("parse_mode") != "HTML" || r.FormValue("text") != englishTexts.greeting("") {
			t.Errorf("fallback text = %q in mode %q", r.FormValue("text"), r.FormValue("parse_mode"))
		}
		if !strings.Contains(r.FormValue("reply_markup"), englishTexts.openApp) {
			t.Errorf("fallback lost the app button: %s", r.FormValue("reply_markup"))
		}
	default:
		t.Fatal("the greeting did not fall back to text")
	}
	if got := s.greetingPhotos.get("en"); got != "" {
		t.Errorf("a failed upload cached file_id %q", got)
	}
}

// A message the bot does not understand gets the greeting in words: a picture
// on every stray message would flood the chat.
func TestUnderstoodNothingGetsTheGreetingAsText(t *testing.T) {
	s := newSiteService(site)

	reply, ok := s.handleText(withLanguage(newPlainUpdate("привет"), "ru")).(textReply)
	if !ok {
		t.Fatal("a stray message is not answered with text")
	}
	if reply.params.ParseMode != models.ParseModeHTML || !strings.Contains(reply.params.Text, "<code>марафон 3:30</code>") {
		t.Errorf("reply = %q in mode %q, want the HTML greeting", reply.params.Text, reply.params.ParseMode)
	}
}
