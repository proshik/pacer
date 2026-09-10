package rest

import (
	"encoding/json"
	"fmt"
	"github.com/go-telegram/bot/models"
	"github.com/klauspost/compress/gzhttp"
	"gorun/pkg/calculator"
	"gorun/pkg/telegram"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// calcResponse is the public shape of a calculation result. Both the ready to
// print form and the split parts are returned so callers do not reimplement the
// formatting.
type calcResponse struct {
	Result       string `json:"result"`
	TotalSeconds int    `json:"total_seconds"`
	Hours        int    `json:"hours"`
	Minutes      int    `json:"minutes"`
	Seconds      int    `json:"seconds"`
}

type errorResponse struct {
	Error   string            `json:"error"`
	Details map[string]string `json:"details,omitempty"`
}

func NewHandler(
	debugMode bool,
	tgToken string,
	t *telegram.Service,
	c calculator.Engine,
	a fs.FS,
) (*http.ServeMux, error) {
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/healthz", healthCheck)

	// The calculation API is part of the product, not a debug aid: the browser
	// front end and any external caller reach it in every mode.
	serveMux.HandleFunc("/api/v1/time", calculateTime(c))
	serveMux.HandleFunc("/api/v1/pace", calculatePace(c))
	serveMux.HandleFunc("/api/v1/splits", handleSplits)
	serveMux.HandleFunc("/api/v1/predict", handlePredict)
	serveMux.HandleFunc("/api/v1/vdot", handleVDOT)

	if !debugMode {
		// handle telegram web hook messages
		serveMux.HandleFunc(fmt.Sprintf("/%s", tgToken), handleWebHook(t))
	}

	stripped, err := fs.Sub(a, "assets")
	if err != nil {
		return nil, fmt.Errorf("open embedded assets subfs: %w", err)
	}

	assetsDir := gzhttp.GzipHandler(http.FileServer(http.FS(stripped)))
	serveMux.Handle("/", assetsDir)

	return serveMux, nil
}

// calculateTime GET /api/v1/time?pace=4m50s&dist=21095
func calculateTime(c calculator.Engine) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		details := map[string]string{}
		dist := parseDistance(query.Get("dist"), details)
		paceDuration := parseDurationField(query.Get("pace"), "pace", details)

		if len(details) > 0 {
			writeJSONError(w, http.StatusBadRequest, "validation failed", details)
			return
		}

		writeCalcResult(w, c.Time(dist, paceDuration))
	}
}

// calculatePace GET /api/v1/pace?dist=21097&time=1h38m48s
func calculatePace(c calculator.Engine) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		details := map[string]string{}
		dist := parseDistance(query.Get("dist"), details)
		timeDuration := parseDurationField(query.Get("time"), "time", details)

		if len(details) > 0 {
			writeJSONError(w, http.StatusBadRequest, "validation failed", details)
			return
		}

		writeCalcResult(w, c.Pace(dist, timeDuration))
	}
}

// parseDistance validates the dist query parameter in meters, recording why it
// was rejected in details. An empty value is reported as missing rather than as
// a malformed number.
func parseDistance(value string, details map[string]string) int {
	if value == "" {
		details["dist"] = "field is required"
		return 0
	}

	dist, err := strconv.Atoi(value)
	if err != nil {
		details["dist"] = "incorrect type should be number"
		return 0
	}

	if dist <= 0 {
		details["dist"] = "value should be greater than zero"
		return 0
	}

	return dist
}

// parseDurationField validates a Go duration query parameter, recording why it
// was rejected under its own field name in details.
func parseDurationField(value string, field string, details map[string]string) time.Duration {
	if value == "" {
		details[field] = "field is required"
		return 0
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		details[field] = err.Error()
		return 0
	}

	return duration
}

func handleWebHook(t *telegram.Service) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()

		data, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Warn("read webhook body failed", "err", err, "remote_addr", r.RemoteAddr)
			writeJSONError(w, http.StatusBadRequest, "invalid request body", nil)
			return
		}

		var update models.Update
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

		t.DoUpdate(&update)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

func writeCalcResult(w http.ResponseWriter, result time.Duration) {
	hours, minutes, seconds := calculator.Split(result)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(calcResponse{
		Result:       result.String(),
		TotalSeconds: int(result.Seconds()),
		Hours:        hours,
		Minutes:      minutes,
		Seconds:      seconds,
	}); err != nil {
		slog.Error("encode calculation result failed", "err", err)
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
