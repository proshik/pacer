package analysis

import (
	"math"
	"testing"
	"time"
)

func TestNewDuration(t *testing.T) {
	got := NewDuration(time.Hour + 31*time.Minute + 35*time.Second)

	if want := (Duration{Text: "1h31m35s", Seconds: 5495}); got != want {
		t.Errorf("NewDuration = %+v, want %+v", got, want)
	}
}

func TestSplitsEveryKilometer(t *testing.T) {
	got := Splits(10000, 5*time.Minute, 1000)

	if len(got.Splits) != 10 {
		t.Fatalf("len(splits) = %d, want 10", len(got.Splits))
	}

	want := Split{Distance: 10000, Elapsed: Duration{Text: "50m0s", Seconds: 3000}}
	if last := got.Splits[len(got.Splits)-1]; last != want {
		t.Errorf("last split = %+v, want %+v", last, want)
	}
}

func TestPredictCoversStandardDistances(t *testing.T) {
	got := Predict(5000, 20*time.Minute)

	if len(got.Predictions) != len(StandardDistances) {
		t.Fatalf("len(predictions) = %d, want %d", len(got.Predictions), len(StandardDistances))
	}

	for i, want := range StandardDistances {
		if got.Predictions[i].Distance != want {
			t.Errorf("prediction %d distance = %d, want %d", i, got.Predictions[i].Distance, want)
		}
	}
}

func TestVDOTAnalysisForPublishedResult(t *testing.T) {
	// 19:57 over 5 km is VDOT 50 in Daniels' published table.
	got := VDOTAnalysis(5000, 19*time.Minute+57*time.Second)

	if math.Abs(got.VDOT-50) > 0.3 {
		t.Errorf("vdot = %.2f, want 50 ±0.3", got.VDOT)
	}

	if tenths := got.VDOT * 10; math.Abs(tenths-math.Round(tenths)) > 1e-9 {
		t.Errorf("vdot = %v, want it rounded to one decimal", got.VDOT)
	}

	if len(got.Equivalents) != len(StandardDistances) {
		t.Errorf("len(equivalents) = %d, want %d", len(got.Equivalents), len(StandardDistances))
	}

	// Published VDOT 50 threshold pace: 4'15 per kilometer.
	if diff := got.Paces.Threshold.Seconds - 255; diff < -3 || diff > 3 {
		t.Errorf("threshold pace = %d s/km, want 255 ±3", got.Paces.Threshold.Seconds)
	}
}
