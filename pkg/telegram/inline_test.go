package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"gorun/pkg/plan"
)

// Exercise routing, worker lifecycle and the library's actual wire format.
func TestInlineQueryReachesTelegram(t *testing.T) {
	for _, tc := range []struct{ query, language, offset, site, want string }{
		{"марафон 3:30", "ru", "", site, "Марафон за 3:30:00 — это 4:59 на километр"},
		{"10k 4:50", "en", "", site, "10 km in 48:20 — that's 4:50 per kilometer"},
		{"", "ru", "", site, ""}, {"junk", "en", "", site, ""},
		{"марафон 3:30", "ru", "next", site, ""},
		{"марафон 3:30", "ru", "", "", ""},
		{"1001km 5:00/km", "ru", "", site, ""},
	} {
		t.Run(tc.query+tc.language+tc.offset+tc.site, func(t *testing.T) {
			received := make(chan map[string]string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/answerInlineQuery") {
					t.Error(r.URL.Path)
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
				}
				received <- map[string]string{"id": r.FormValue("inline_query_id"), "results": r.FormValue("results"), "personal": r.FormValue("is_personal")}
				_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			}))
			defer server.Close()
			client, err := bot.New("test", bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
			if err != nil {
				t.Fatal(err)
			}
			s := newService()
			s.siteURL = tc.site
			s.bot = client
			s.startDispatcher()
			s.startSender()
			defer func() { _ = s.Close(context.Background()) }()
			s.DoUpdate(&models.Update{InlineQuery: &models.InlineQuery{ID: "inline-1", From: &models.User{LanguageCode: tc.language}, Query: tc.query, Offset: tc.offset}})
			select {
			case got := <-received:
				if got["id"] != "inline-1" || got["personal"] != "true" {
					t.Fatal(got)
				}
				var results []map[string]any
				if err := json.Unmarshal([]byte(got["results"]), &results); err != nil {
					t.Fatal(err)
				}
				if tc.want == "" {
					if len(results) != 0 {
						t.Fatal(results)
					}
					return
				}
				if len(results) != 1 {
					t.Fatal(results)
				}
				if results[0]["type"] != "article" {
					t.Fatalf("wrong Telegram result type: %v", results[0])
				}
				content := results[0]["input_message_content"].(map[string]any)
				if content["message_text"] != tc.want {
					t.Errorf("message text: %v", content)
				}
				markup := results[0]["reply_markup"].(map[string]any)
				rows := markup["inline_keyboard"].([]any)
				link := rows[0].([]any)[0].(map[string]any)["url"].(string)
				parsed, err := url.Parse(link)
				if err != nil || parsed.Fragment != "" || parsed.Query().Get("l") != tc.language || len(parsed.Query()) != 3 {
					t.Errorf("unsafe plan link: %s", link)
				}
				if tc.language == "ru" && (parsed.Query().Get("d") != "42195" || parsed.Query().Get("t") != "3:30:00") {
					t.Error(link)
				}
				data, _ := json.Marshal(results[0])
				text := string(data)
				if !strings.Contains(text, tc.want) || !strings.Contains(text, "https://pacer.example.com/") || strings.Contains(text, "web_app") || strings.Contains(text, "callback_data") {
					t.Fatal(text)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("inline query was not answered")
			}
		})
	}
}

func TestPrepareInlineMessageBindsUserAndReturnsTelegramID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/savePreparedInlineMessage") {
			t.Error(r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		if r.FormValue("user_id") != "42" || r.FormValue("allow_user_chats") != "true" || r.FormValue("allow_group_chats") != "true" || r.FormValue("allow_channel_chats") != "true" || r.FormValue("allow_bot_chats") == "true" {
			t.Error(r.Form)
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(r.FormValue("result")), &result); err != nil {
			t.Error(err)
		}
		if !strings.Contains(r.FormValue("result"), "Марафон за 3:30:00") {
			t.Error(result)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":"prepared-1","expiration_date":1900000000}}`))
	}))
	defer server.Close()
	client, err := bot.New("test", bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	s := newSiteService(site)
	s.bot = client
	got, err := s.PrepareInlineMessage(context.Background(), 42, plan.Plan{Distance: 42195, Time: 210 * time.Minute}, "ru")
	if err != nil || got.ID != "prepared-1" || got.ExpirationDate != 1900000000 {
		t.Fatalf("%+v %v", got, err)
	}
}
