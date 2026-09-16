package telegram

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const site = "https://pacer.example.com"

func newSiteService(siteURL string) *Service {
	s := newService()
	s.siteURL = siteURL
	return s
}

func textParams(t *testing.T, reply outgoing) *bot.SendMessageParams {
	t.Helper()

	text, ok := reply.(textReply)
	if !ok {
		t.Fatalf("expected a text reply, got %T", reply)
	}

	return text.params
}

func buttonsOf(t *testing.T, markup models.ReplyMarkup) []models.InlineKeyboardButton {
	t.Helper()

	if markup == nil {
		return nil
	}

	keyboard, ok := markup.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("reply markup is %T, want an inline keyboard", markup)
	}

	var buttons []models.InlineKeyboardButton
	for _, row := range keyboard.InlineKeyboard {
		buttons = append(buttons, row...)
	}

	return buttons
}

func buttonMap(buttons []models.InlineKeyboardButton) map[string]string {
	out := map[string]string{}
	for _, b := range buttons {
		switch {
		case b.WebApp != nil:
			out[b.Text] = "web_app " + b.WebApp.URL
		default:
			out[b.Text] = b.CallbackData
		}
	}

	return out
}

func newCallbackUpdate(data, language string) *models.Update {
	return &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:   "query-1",
		From: models.User{ID: 7, LanguageCode: language},
		Message: models.MaybeInaccessibleMessage{
			Type:    models.MaybeInaccessibleMessageTypeMessage,
			Message: &models.Message{ID: 99, Chat: models.Chat{ID: 42, Type: models.ChatTypePrivate}},
		},
		Data: data,
	}}
}

func callbackOf(t *testing.T, reply outgoing) callbackReply {
	t.Helper()

	cb, ok := reply.(callbackReply)
	if !ok {
		t.Fatalf("expected a callback reply, got %T", reply)
	}
	if cb.answer == nil || cb.answer.CallbackQueryID != "query-1" {
		t.Fatalf("the tap is not answered: %+v", cb.answer)
	}

	return cb
}

// A command in a runner's own words answers with the plan and the ways on:
// the splits, the prediction, the card, and the app opened on this plan.
func TestPlanCommandAnswersWithThePlanAndButtons(t *testing.T) {
	s := newSiteService(site)
	params := textParams(t, s.handlePaceCmd(withLanguage(newCommandUpdate("/pace 21.1 1:38:48"), "ru")))

	if want := "21,1 км за 1:38:48 — это 4:41 на километр"; !strings.Contains(params.Text, want) {
		t.Errorf("reply %q does not say %q", params.Text, want)
	}
	if params.ParseMode != models.ParseModeHTML {
		t.Errorf("parse mode = %q, want HTML", params.ParseMode)
	}

	got := buttonMap(buttonsOf(t, params.ReplyMarkup))
	want := map[string]string{
		"Раскладка":       "splits:21100:5928",
		"Прогноз":         "predict:21100:5928",
		"Карточка":        "card:21100:5928",
		"Открыть в Pacer": "web_app " + site + "/?d=21100&t=1:38:48&l=ru",
	}
	for text, action := range want {
		if got[text] != action {
			t.Errorf("button %q = %q, want %q (all: %v)", text, got[text], action, got)
		}
	}
}

func TestTimeCommandTakesAPace(t *testing.T) {
	s := newSiteService(site)
	params := textParams(t, s.handleTimeCmd(newCommandUpdate("/time 4:50 10k")))

	if want := "10 km in 48:20 — that's 4:50 per kilometer"; !strings.Contains(params.Text, want) {
		t.Errorf("reply %q does not say %q", params.Text, want)
	}
	if got := buttonMap(buttonsOf(t, params.ReplyMarkup)); got["Splits"] != "splits:10000:2900" {
		t.Errorf("English buttons = %v, want Splits for 10000 m in 2900 s", got)
	}
}

func TestOldCommandSyntaxStillWorks(t *testing.T) {
	s := newSiteService("")

	if text := textParams(t, s.handlePaceCmd(newCommandUpdate("/pace 21097 1h38m48s"))).Text; !strings.Contains(text, "Half marathon in 1:38:48") {
		t.Errorf("/pace 21097 1h38m48s = %q", text)
	}
	if text := textParams(t, s.handleTimeCmd(newCommandUpdate("/time 4m50s 21095"))).Text; !strings.Contains(text, "21.095 km in 1:41:57") {
		t.Errorf("/time 4m50s 21095 = %q", text)
	}
}

// Without an https site Telegram would refuse a web_app button, so there is
// none; the other buttons stay.
func TestNoAppButtonWithoutAnHTTPSSite(t *testing.T) {
	s := newSiteService("")
	got := buttonMap(buttonsOf(t, textParams(t, s.handlePaceCmd(newCommandUpdate("/pace 10k 50:00"))).ReplyMarkup))

	for text, action := range got {
		if strings.HasPrefix(action, "web_app") {
			t.Errorf("button %q opens %q without an https site", text, action)
		}
	}
	if got["Splits"] == "" {
		t.Errorf("the other buttons are gone too: %v", got)
	}
}

// Telegram accepts web_app buttons in private chats only; a group gets a plain
// link to the site instead.
func TestGroupChatGetsALinkInsteadOfTheApp(t *testing.T) {
	s := newSiteService(site)

	inGroup := func(update *models.Update) *models.Update {
		update.Message.Chat.Type = models.ChatTypeGroup
		return update
	}

	for name, reply := range map[string]outgoing{
		"plan":  s.handlePaceCmd(inGroup(newCommandUpdate("/pace 10k 50:00"))),
		"start": s.handleStartCmd(inGroup(newCommandUpdate("/start"))),
	} {
		markup, ok := textParams(t, reply).ReplyMarkup.(models.InlineKeyboardMarkup)
		if !ok {
			t.Fatalf("%s: reply markup = %T, want an inline keyboard", name, textParams(t, reply).ReplyMarkup)
		}

		var links int
		for _, row := range markup.InlineKeyboard {
			for _, button := range row {
				if button.WebApp != nil {
					t.Errorf("%s: button %q opens a web app in a group", name, button.Text)
				}
				if strings.HasPrefix(button.URL, site+"/") {
					links++
				}
			}
		}
		if links != 1 {
			t.Errorf("%s: %d links to the site, want 1: %+v", name, links, markup.InlineKeyboard)
		}
	}
}

func TestWebAppSite(t *testing.T) {
	tests := []struct{ host, want string }{
		{"pacer.example.com", "https://pacer.example.com"},
		{"https://pacer.example.com/", "https://pacer.example.com"},
		{"https://example.com/pacer/", "https://example.com/pacer"},
		{"http://localhost:8099", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := webAppSite(tt.host); got != tt.want {
			t.Errorf("webAppSite(%q) = %q, want %q", tt.host, got, tt.want)
		}
	}
}

func TestPlainTextIsReadAsAPlan(t *testing.T) {
	s := newTestService(t)
	s.siteURL = site

	s.handleUpdate(withLanguage(newPlainUpdate("марафон 3:30"), "ru"))

	if text := messageText(t, receiveMessage(t, s)); !strings.Contains(text, "Марафон за 3:30:00") {
		t.Errorf("plain text reply = %q, want the marathon plan", text)
	}
}

func TestStartOffersToOpenTheApp(t *testing.T) {
	s := newTestService(t)
	s.siteURL = site

	s.handleUpdate(withLanguage(newCommandUpdate("/start"), "ru"))

	params := textParams(t, receiveMessage(t, s))
	if got := buttonMap(buttonsOf(t, params.ReplyMarkup)); got["Открыть Pacer"] != "web_app "+site+"/" {
		t.Errorf("start buttons = %v, want one that opens %s/", got, site)
	}
	if !strings.Contains(params.Text, "марафон 3:30") {
		t.Errorf("greeting %q does not show a plain example", params.Text)
	}
}

func TestCallbackShowsSplitsInTheSameMessage(t *testing.T) {
	s := newSiteService(site)
	cb := callbackOf(t, s.handleCallback(newCallbackUpdate("splits:21097:5928", "ru")))

	if cb.edit == nil {
		t.Fatal("the message is not rewritten")
	}
	if cb.edit.ChatID != int64(42) || cb.edit.MessageID != 99 {
		t.Errorf("edits chat %v message %d, want 42 and 99", cb.edit.ChatID, cb.edit.MessageID)
	}
	for _, want := range []string{"<pre>", "1 км", "21 км", "финиш", "1:38:48", "Полумарафон за 1:38:48"} {
		if !strings.Contains(cb.edit.Text, want) {
			t.Errorf("splits %q lack %q", cb.edit.Text, want)
		}
	}

	got := buttonMap(buttonsOf(t, cb.edit.ReplyMarkup))
	if got["← План"] != "plan:21097:5928" || got["Прогноз"] == "" || got["Раскладка"] != "" {
		t.Errorf("buttons under the splits = %v, want back, prediction and card", got)
	}
}

func TestCallbackShowsThePrediction(t *testing.T) {
	s := newSiteService(site)
	cb := callbackOf(t, s.handleCallback(newCallbackUpdate("predict:21097:5928", "ru")))

	if cb.edit == nil {
		t.Fatal("the message is not rewritten")
	}
	for _, want := range []string{"<pre>", "Ригель", "Кэмерон", "Марафон", "5 км"} {
		if !strings.Contains(cb.edit.Text, want) {
			t.Errorf("prediction %q lacks %q", cb.edit.Text, want)
		}
	}
}

func TestCallbackGoesBackToThePlan(t *testing.T) {
	s := newSiteService(site)
	cb := callbackOf(t, s.handleCallback(newCallbackUpdate("plan:21097:5928", "en")))

	if cb.edit == nil || !strings.Contains(cb.edit.Text, "Half marathon in 1:38:48") {
		t.Fatalf("back to the plan = %+v", cb.edit)
	}
	if got := buttonMap(buttonsOf(t, cb.edit.ReplyMarkup)); got["Splits"] == "" || got["← Plan"] != "" {
		t.Errorf("plan buttons = %v, want the plan's own set", got)
	}
}

func TestCallbackSendsTheCard(t *testing.T) {
	s := newSiteService(site)
	cb := callbackOf(t, s.handleCallback(newCallbackUpdate("card:21097:5928", "ru")))

	if cb.photo == nil || cb.edit != nil {
		t.Fatalf("card tap = photo %v, edit %v; want a photo only", cb.photo, cb.edit)
	}
	if !strings.Contains(cb.photo.Caption, "Полумарафон за 1:38:48") {
		t.Errorf("caption = %q", cb.photo.Caption)
	}
}

// Callback data comes from the client and can be anything; a tap that does not
// parse is answered and changes nothing.
func TestBrokenCallbackIsOnlyAnswered(t *testing.T) {
	s := newSiteService(site)

	for _, data := range []string{"", "splits", "splits:abc:1", "splits:0:1", "delete:21097:5928", "splits:99999999:1"} {
		cb := callbackOf(t, s.handleCallback(newCallbackUpdate(data, "ru")))
		if cb.edit != nil || cb.photo != nil {
			t.Errorf("data %q changed something: edit %v photo %v", data, cb.edit, cb.photo)
		}
	}
}
