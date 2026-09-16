package rest

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gorun/pkg/miniapp"
)

// initDataMaxAge is how long Mini App launch data stays valid. Telegram signs
// it when the app opens, so a session left open all day keeps working, while a
// leaked string stops working by the next day.
const initDataMaxAge = 24 * time.Hour

type meResponse struct {
	User miniapp.User `json:"user"`
}

// handleMe GET /api/v1/me with "Authorization: tma <init data>" answers who
// opened the Mini App.
func handleMe(botToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, ok := authenticate(w, r, botToken)
		if !ok {
			return
		}

		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, meResponse{User: data.User})
	}
}

// authenticate validates the Mini App init data sent in the Authorization
// header, answering 401 itself when the data is missing, forged or stale.
func authenticate(w http.ResponseWriter, r *http.Request, botToken string) (miniapp.InitData, bool) {
	raw, found := strings.CutPrefix(r.Header.Get("Authorization"), "tma ")
	if !found || raw == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing init data", nil)
		return miniapp.InitData{}, false
	}

	data, err := miniapp.ValidateInitData(raw, botToken, initDataMaxAge, time.Now())
	switch {
	case err == nil:
		// What a live launch settles and a test cannot: which language the
		// client reports and whether it sends the signature field.
		values, _ := url.ParseQuery(raw)
		slog.Debug("mini app sign-in", "language_code", data.User.LanguageCode, "signature", values.Has("signature"))
		return data, true
	case errors.Is(err, miniapp.ErrExpired):
		// Genuine but old: reopening the app makes Telegram sign fresh data.
		writeJSONError(w, http.StatusUnauthorized, "init data expired, reopen the app", nil)
	default:
		slog.Debug("mini app init data rejected", "err", err)
		writeJSONError(w, http.StatusUnauthorized, "invalid init data", nil)
	}

	return miniapp.InitData{}, false
}
