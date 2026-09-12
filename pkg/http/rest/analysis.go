package rest

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"gorun/pkg/analysis"
)

// handleSplits GET /api/v1/splits?dist=21097&pace=4m30s&step=1000
func handleSplits(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	details := map[string]string{}
	dist := parseDistance(query.Get("dist"), details)
	pace := parsePositiveDurationField(query.Get("pace"), "pace", details)
	step := parseStep(query.Get("step"), details)

	if len(details) == 0 && dist/step > analysis.MaxSplits {
		details["step"] = "too many splits, use a larger step"
	}

	if len(details) > 0 {
		writeJSONError(w, http.StatusBadRequest, "validation failed", details)
		return
	}

	writeJSON(w, analysis.Splits(dist, pace, step))
}

// handlePredict GET /api/v1/predict?dist=5000&time=20m0s
func handlePredict(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	details := map[string]string{}
	dist := parseDistance(query.Get("dist"), details)
	raceTime := parsePositiveDurationField(query.Get("time"), "time", details)

	if len(details) > 0 {
		writeJSONError(w, http.StatusBadRequest, "validation failed", details)
		return
	}

	writeJSON(w, analysis.Predict(dist, raceTime))
}

// handleVDOT GET /api/v1/vdot?dist=5000&time=19m57s
func handleVDOT(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	details := map[string]string{}
	dist := parseDistance(query.Get("dist"), details)
	raceTime := parsePositiveDurationField(query.Get("time"), "time", details)

	if len(details) > 0 {
		writeJSONError(w, http.StatusBadRequest, "validation failed", details)
		return
	}

	writeJSON(w, analysis.VDOTAnalysis(dist, raceTime))
}

// parseStep reads the optional marker spacing in meters; one kilometer by default.
func parseStep(value string, details map[string]string) int {
	if value == "" {
		return 1000
	}

	step, err := strconv.Atoi(value)
	if err != nil {
		details["step"] = "incorrect type should be number"
		return 0
	}

	if step <= 0 {
		details["step"] = "value should be greater than zero"
		return 0
	}

	return step
}

// parsePositiveDurationField is parseDurationField that also rejects zero and
// negative durations, which none of the analysis formulas can use.
func parsePositiveDurationField(value string, field string, details map[string]string) time.Duration {
	duration := parseDurationField(value, field, details)
	if _, rejected := details[field]; rejected {
		return 0
	}

	if duration <= 0 {
		details[field] = "value should be greater than zero"
		return 0
	}

	return duration
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("encode json response failed", "err", err)
	}
}
