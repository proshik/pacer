package rest

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"gorun/pkg/calculator"
)

// maxSplits bounds a single /splits response: 100 km in 10 m steps would
// otherwise serialize 10 000 rows for one request.
const maxSplits = 500

// predictionDistances are the race distances /predict and /vdot report, in meters.
var predictionDistances = []int{1000, 3000, 5000, 10000, 21097, 42195}

// durationValue carries a duration both in Go's text form, matching the result
// field of /time and /pace, and as whole seconds for arithmetic.
type durationValue struct {
	Text    string `json:"text"`
	Seconds int    `json:"seconds"`
}

func newDurationValue(duration time.Duration) durationValue {
	return durationValue{Text: duration.String(), Seconds: int(duration.Seconds())}
}

type splitJSON struct {
	Distance int           `json:"distance"`
	Elapsed  durationValue `json:"elapsed"`
}

type splitsResponse struct {
	Splits []splitJSON `json:"splits"`
}

type predictionJSON struct {
	Distance int           `json:"distance"`
	Riegel   durationValue `json:"riegel"`
	Cameron  durationValue `json:"cameron"`
}

type predictResponse struct {
	Predictions []predictionJSON `json:"predictions"`
}

type pacesJSON struct {
	EasySlow  durationValue `json:"easy_slow"`
	EasyFast  durationValue `json:"easy_fast"`
	Marathon  durationValue `json:"marathon"`
	Threshold durationValue `json:"threshold"`
	Interval  durationValue `json:"interval"`
}

type equivalentJSON struct {
	Distance int           `json:"distance"`
	Time     durationValue `json:"time"`
}

type vdotResponse struct {
	VDOT        float64          `json:"vdot"`
	Paces       pacesJSON        `json:"paces"`
	Equivalents []equivalentJSON `json:"equivalents"`
}

// handleSplits GET /api/v1/splits?dist=21097&pace=4m30s&step=1000
func handleSplits(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	details := map[string]string{}
	dist := parseDistance(query.Get("dist"), details)
	pace := parsePositiveDurationField(query.Get("pace"), "pace", details)
	step := parseStep(query.Get("step"), details)

	if len(details) == 0 && dist/step > maxSplits {
		details["step"] = "too many splits, use a larger step"
	}

	if len(details) > 0 {
		writeJSONError(w, http.StatusBadRequest, "validation failed", details)
		return
	}

	points := calculator.EvenSplits(dist, pace, step)
	splits := make([]splitJSON, 0, len(points))
	for _, point := range points {
		splits = append(splits, splitJSON{Distance: point.Distance, Elapsed: newDurationValue(point.Elapsed)})
	}

	writeJSON(w, splitsResponse{Splits: splits})
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

	predictions := make([]predictionJSON, 0, len(predictionDistances))
	for _, target := range predictionDistances {
		predictions = append(predictions, predictionJSON{
			Distance: target,
			Riegel:   newDurationValue(calculator.PredictRiegel(dist, raceTime, target)),
			Cameron:  newDurationValue(calculator.PredictCameron(dist, raceTime, target)),
		})
	}

	writeJSON(w, predictResponse{Predictions: predictions})
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

	vdot := calculator.VDOT(dist, raceTime)
	paces := calculator.TrainingPacesForVDOT(vdot)

	// Equivalents use the unrounded VDOT; only the reported figure is rounded.
	equivalents := make([]equivalentJSON, 0, len(predictionDistances))
	for _, target := range predictionDistances {
		equivalents = append(equivalents, equivalentJSON{
			Distance: target,
			Time:     newDurationValue(calculator.RaceTimeForVDOT(vdot, target)),
		})
	}

	writeJSON(w, vdotResponse{
		VDOT: math.Round(vdot*10) / 10,
		Paces: pacesJSON{
			EasySlow:  newDurationValue(paces.EasySlow),
			EasyFast:  newDurationValue(paces.EasyFast),
			Marathon:  newDurationValue(paces.Marathon),
			Threshold: newDurationValue(paces.Threshold),
			Interval:  newDurationValue(paces.Interval),
		},
		Equivalents: equivalents,
	})
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
