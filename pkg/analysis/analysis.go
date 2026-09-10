// Package analysis shapes calculator results for display.
//
// The HTTP API and the browser wasm module both return these exact
// structures, so a split table or a VDOT readout is identical whether it came
// from the server or was computed in the page.
package analysis

import (
	"math"
	"time"

	"gorun/pkg/calculator"
)

// MaxSplits bounds one split table: 100 km in 10 m steps would otherwise be
// 10 000 rows.
const MaxSplits = 500

// StandardDistances are the race distances that predictions and VDOT
// equivalents are reported for, in meters.
var StandardDistances = []int{1000, 3000, 5000, 10000, 21097, 42195}

// Duration carries a duration in Go's text form and as whole seconds.
type Duration struct {
	Text    string `json:"text"`
	Seconds int    `json:"seconds"`
}

func NewDuration(duration time.Duration) Duration {
	return Duration{Text: duration.String(), Seconds: int(duration.Seconds())}
}

type Split struct {
	Distance int      `json:"distance"`
	Elapsed  Duration `json:"elapsed"`
}

type SplitsResult struct {
	Splits []Split `json:"splits"`
}

// Splits is an even-pace split table with a marker every step meters.
func Splits(dist int, pace time.Duration, step int) SplitsResult {
	points := calculator.EvenSplits(dist, pace, step)

	splits := make([]Split, 0, len(points))
	for _, point := range points {
		splits = append(splits, Split{Distance: point.Distance, Elapsed: NewDuration(point.Elapsed)})
	}

	return SplitsResult{Splits: splits}
}

type Prediction struct {
	Distance int      `json:"distance"`
	Riegel   Duration `json:"riegel"`
	Cameron  Duration `json:"cameron"`
}

type PredictResult struct {
	Predictions []Prediction `json:"predictions"`
}

// Predict estimates every standard distance from one known result, with both
// the Riegel and the Cameron model.
func Predict(dist int, raceTime time.Duration) PredictResult {
	predictions := make([]Prediction, 0, len(StandardDistances))
	for _, target := range StandardDistances {
		predictions = append(predictions, Prediction{
			Distance: target,
			Riegel:   NewDuration(calculator.PredictRiegel(dist, raceTime, target)),
			Cameron:  NewDuration(calculator.PredictCameron(dist, raceTime, target)),
		})
	}

	return PredictResult{Predictions: predictions}
}

type Paces struct {
	EasySlow  Duration `json:"easy_slow"`
	EasyFast  Duration `json:"easy_fast"`
	Marathon  Duration `json:"marathon"`
	Threshold Duration `json:"threshold"`
	Interval  Duration `json:"interval"`
}

type Equivalent struct {
	Distance int      `json:"distance"`
	Time     Duration `json:"time"`
}

type VDOTResult struct {
	VDOT        float64      `json:"vdot"`
	Paces       Paces        `json:"paces"`
	Equivalents []Equivalent `json:"equivalents"`
}

// VDOTAnalysis derives VDOT from one race result, with training paces per
// kilometer and equivalent times for the standard distances.
func VDOTAnalysis(dist int, raceTime time.Duration) VDOTResult {
	vdot := calculator.VDOT(dist, raceTime)
	paces := calculator.TrainingPacesForVDOT(vdot)

	// Equivalents use the unrounded VDOT; only the reported figure is rounded.
	equivalents := make([]Equivalent, 0, len(StandardDistances))
	for _, target := range StandardDistances {
		equivalents = append(equivalents, Equivalent{
			Distance: target,
			Time:     NewDuration(calculator.RaceTimeForVDOT(vdot, target)),
		})
	}

	return VDOTResult{
		VDOT: math.Round(vdot*10) / 10,
		Paces: Paces{
			EasySlow:  NewDuration(paces.EasySlow),
			EasyFast:  NewDuration(paces.EasyFast),
			Marathon:  NewDuration(paces.Marathon),
			Threshold: NewDuration(paces.Threshold),
			Interval:  NewDuration(paces.Interval),
		},
		Equivalents: equivalents,
	}
}
