package rest

import (
	"encoding/json"
	"fmt"
	"github.com/NYTimes/gziphandler"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"gorun/pkg/calculator"
	"gorun/pkg/telegram"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

type errorResponse struct {
	Error   string            `json:"error"`
	Details map[string]string `json:"details,omitempty"`
}

func NewHandler(
	debugMode bool,
	tgToken string,
	t *telegram.Service,
	c *calculator.Service,
	a fs.FS,
) (*http.ServeMux, error) {
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/healthz", healthCheck)

	if debugMode {
		// debug endpoints
		serveMux.HandleFunc("/time", calculateTime(c))
		serveMux.HandleFunc("/pace", calculatePace(c))
	} else {
		// handle telegram web hook messages
		serveMux.HandleFunc(fmt.Sprintf("/%s", tgToken), handleWebHook(t))
	}

	stripped, err := fs.Sub(a, "assets")
	if err != nil {
		return nil, fmt.Errorf("open embedded assets subfs: %w", err)
	}

	assetsDir := gziphandler.GzipHandler(http.FileServer(http.FS(stripped)))
	serveMux.Handle("/", assetsDir)

	return serveMux, nil
}

// calculateTime http://localhost:8080/time?pace=4m50s&dist=21095
func calculateTime(c *calculator.Service) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		details := map[string]string{}
		distValue := query.Get("dist")
		if distValue == "" {
			details["dist"] = "field is required"
		}

		paceValue := query.Get("pace")
		if paceValue == "" {
			details["pace"] = "field is required"
		}

		dist, distErr := strconv.Atoi(distValue)
		if distErr != nil {
			details["dist"] = "incorrect type should be number"
		} else if dist <= 0 {
			details["dist"] = "value should be greater than zero"
		}

		paceDuration, paceErr := time.ParseDuration(paceValue)
		if paceErr != nil {
			details["pace"] = paceErr.Error()
		}

		if len(details) > 0 {
			writeJSONError(w, http.StatusBadRequest, "validation failed", details)
			return
		}

		result := c.Time(dist, paceDuration)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(result.String()))
	}
}

// calculatePace http://localhost:8080/pace?dist=21097&time=1h38m48s
func calculatePace(c *calculator.Service) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		details := map[string]string{}
		distValue := query.Get("dist")
		if distValue == "" {
			details["dist"] = "field is required"
		}

		timeValue := query.Get("time")
		if timeValue == "" {
			details["time"] = "field is required"
		}

		dist, distErr := strconv.Atoi(distValue)
		if distErr != nil {
			details["dist"] = "incorrect type should be number"
		} else if dist <= 0 {
			details["dist"] = "value should be greater than zero"
		}

		timeDuration, timeErr := time.ParseDuration(timeValue)
		if timeErr != nil {
			details["time"] = timeErr.Error()
		}

		if len(details) > 0 {
			writeJSONError(w, http.StatusBadRequest, "validation failed", details)
			return
		}

		result := c.Pace(dist, timeDuration)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(result.String()))
	}
}

func handleWebHook(t *telegram.Service) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		data, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Warn("read webhook body failed", "err", err, "remote_addr", r.RemoteAddr)
			writeJSONError(w, http.StatusBadRequest, "invalid request body", nil)
			return
		}

		var update tgbotapi.Update
		if err = json.Unmarshal(data, &update); err != nil {
			slog.Warn("decode webhook update failed", "err", err, "remote_addr", r.RemoteAddr)
			writeJSONError(w, http.StatusBadRequest, "invalid telegram update payload", nil)
			return
		}

		if t == nil {
			slog.Error("telegram service unavailable while handling webhook")
			writeJSONError(w, http.StatusServiceUnavailable, "telegram service unavailable", nil)
			return
		}

		t.DoUpdate(update)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

func healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func writeJSONError(w http.ResponseWriter, statusCode int, message string, details map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(errorResponse{
		Error:   message,
		Details: details,
	}); err != nil {
		slog.Error("encode json error response failed", "err", err, "status_code", statusCode)
	}
}
