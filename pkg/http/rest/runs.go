package rest

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"gorun/pkg/analysis"
	"gorun/pkg/history"
)

// runsListLimit bounds how many saved runs a single list returns.
const runsListLimit = 50

type runResponse struct {
	ID       int64             `json:"id"`
	Distance int               `json:"distance"`
	Time     analysis.Duration `json:"time"`
	SavedAt  string            `json:"saved_at"`
}

func newRunResponse(run history.Run) runResponse {
	return runResponse{
		ID:       run.ID,
		Distance: run.Distance,
		Time:     analysis.NewDuration(run.Time),
		SavedAt:  run.SavedAt.UTC().Format(time.RFC3339),
	}
}

type saveRunRequest struct {
	Distance    int `json:"distance"`
	TimeSeconds int `json:"time_seconds"`
}

// historyAvailable answers 503 itself when no database is configured.
func historyAvailable(w http.ResponseWriter, store *history.Store) bool {
	if store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "history is not configured", nil)
		return false
	}

	return true
}

// listRunsHandler GET /api/v1/runs: the signed-in user's saved runs, newest first.
func listRunsHandler(botToken string, store *history.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !historyAvailable(w, store) {
			return
		}
		session, ok := authenticate(w, r, botToken)
		if !ok {
			return
		}

		runs, err := store.List(r.Context(), session.User.ID, runsListLimit)
		if err != nil {
			slog.Error("list saved runs failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "could not load saved runs", nil)
			return
		}

		response := struct {
			Runs []runResponse `json:"runs"`
		}{Runs: make([]runResponse, 0, len(runs))}
		for _, run := range runs {
			response.Runs = append(response.Runs, newRunResponse(run))
		}

		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, response)
	}
}

// saveRunHandler POST /api/v1/runs {"distance": meters, "time_seconds": seconds}
func saveRunHandler(botToken string, store *history.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !historyAvailable(w, store) {
			return
		}
		session, ok := authenticate(w, r, botToken)
		if !ok {
			return
		}

		var request saveRunRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&request); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", nil)
			return
		}

		details := map[string]string{}
		if request.Distance <= 0 {
			details["distance"] = "value should be greater than zero"
		}
		if request.TimeSeconds <= 0 {
			details["time_seconds"] = "value should be greater than zero"
		}
		if len(details) > 0 {
			writeJSONError(w, http.StatusBadRequest, "validation failed", details)
			return
		}

		finish := time.Duration(request.TimeSeconds) * time.Second
		run, err := store.Save(r.Context(), session.User.ID, request.Distance, finish, time.Now())
		if err != nil {
			slog.Error("save run failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "could not save the run", nil)
			return
		}

		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(struct {
			Run runResponse `json:"run"`
		}{Run: newRunResponse(run)}); err != nil {
			slog.Error("encode saved run failed", "err", err)
		}
	}
}

// deleteRunHandler DELETE /api/v1/runs/{id}
func deleteRunHandler(botToken string, store *history.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !historyAvailable(w, store) {
			return
		}
		session, ok := authenticate(w, r, botToken)
		if !ok {
			return
		}

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid run id", nil)
			return
		}

		switch err := store.Delete(r.Context(), session.User.ID, id); {
		case err == nil:
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, history.ErrNotFound):
			writeJSONError(w, http.StatusNotFound, "run not found", nil)
		default:
			slog.Error("delete run failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "could not delete the run", nil)
		}
	}
}
