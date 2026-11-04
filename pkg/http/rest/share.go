package rest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-telegram/bot/models"

	"gorun/pkg/plan"
	"gorun/pkg/telegram"
)

type inlinePreparer interface {
	PrepareInlineMessage(context.Context, int64, plan.Plan, string) (*models.PreparedInlineMessage, error)
}

func prepareShareHandler(token string, service inlinePreparer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		session, ok := authenticate(w, r, token)
		if !ok {
			return
		}
		if session.User.ID <= 0 {
			writeJSONError(w, http.StatusUnauthorized, "missing telegram user", nil)
			return
		}
		var request struct {
			Distance    int    `json:"distance"`
			TimeSeconds int    `json:"time_seconds"`
			Language    string `json:"language"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", nil)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", nil)
			return
		}
		if request.Distance < 1 || request.Distance > 1000000 || request.TimeSeconds < 1 || request.TimeSeconds > 3600000 || (request.Language != "ru" && request.Language != "en") {
			writeJSONError(w, http.StatusBadRequest, "invalid shared plan", nil)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		prepared, err := service.PrepareInlineMessage(ctx, session.User.ID, plan.Plan{Distance: request.Distance, Time: time.Duration(request.TimeSeconds) * time.Second}, request.Language)
		if errors.Is(err, telegram.ErrSharingUnavailable) {
			writeJSONError(w, http.StatusServiceUnavailable, "telegram sharing is not configured", nil)
			return
		}
		// Telegram errors can contain transport details; keep them out of responses and logs.
		if err != nil || prepared == nil || prepared.ID == "" {
			writeJSONError(w, http.StatusBadGateway, "could not prepare shared message", nil)
			return
		}
		writeJSON(w, prepared)
	}
}
