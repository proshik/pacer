package rest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// signInitData signs Mini App init data the way Telegram does. The published
// example in pkg/miniapp anchors the algorithm; this only produces fresh data,
// since that example was signed in 2022 and is long past any freshness window.
func signInitData(token string, authDate time.Time, user string) string {
	fields := map[string]string{
		"auth_date": strconv.FormatInt(authDate.Unix(), 10),
		"user":      user,
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	values := url.Values{}
	for _, key := range keys {
		lines = append(lines, key+"="+fields[key])
		values.Set(key, fields[key])
	}

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))

	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	values.Set("hash", hex.EncodeToString(mac.Sum(nil)))

	return values.Encode()
}

func requestMe(t *testing.T, authorization string) *httptest.ResponseRecorder {
	t.Helper()

	handler := newTestHandlerWithMode(t, false)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder
}

func TestMeReturnsTheTelegramUser(t *testing.T) {
	// newTestHandlerWithMode builds the handler with the bot token "token".
	initData := signInitData("token", time.Now(), `{"id":42,"first_name":"Ann","language_code":"ru"}`)

	recorder := requestMe(t, "tma "+initData)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %q)", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var body struct {
		User struct {
			ID           int64  `json:"id"`
			FirstName    string `json:"first_name"`
			LanguageCode string `json:"language_code"`
		} `json:"user"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.User.ID != 42 || body.User.FirstName != "Ann" || body.User.LanguageCode != "ru" {
		t.Errorf("user = %+v, want Ann with id 42 and language ru", body.User)
	}
}

func TestMeRejectsRequestsWithoutValidInitData(t *testing.T) {
	const user = `{"id":42,"first_name":"Ann"}`
	fresh := signInitData("token", time.Now(), user)
	stale := signInitData("token", time.Now().Add(-48*time.Hour), user)
	otherBot := signInitData("another-token", time.Now(), user)

	tests := []struct {
		name          string
		authorization string
		message       string
	}{
		{name: "no header", authorization: "", message: "missing init data"},
		{name: "wrong scheme", authorization: "Bearer " + fresh, message: "missing init data"},
		{name: "signed by another bot", authorization: "tma " + otherBot, message: "invalid init data"},
		// Stale data is genuine, just old: reopening the app issues fresh data.
		{name: "stale", authorization: "tma " + stale, message: "init data expired, reopen the app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := requestMe(t, tt.authorization)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
			}

			if got := decodeError(t, recorder.Body).Error; got != tt.message {
				t.Errorf("error = %q, want %q", got, tt.message)
			}
		})
	}
}
