package calculator

import (
	"testing"
	"time"
)

func within(got time.Duration, want time.Duration, tolerance time.Duration) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}

	return diff <= tolerance
}

func TestPredictRiegel(t *testing.T) {
	// 40:00 for 10 km -> 40 * 2.1097^1.06 = 88.254 min, worked by hand.
	got := PredictRiegel(10000, 40*time.Minute, 21097)
	want := time.Hour + 28*time.Minute + 15*time.Second

	if !within(got, want, 2*time.Second) {
		t.Errorf("PredictRiegel(10 km in 40:00 -> half) = %v, want %v ±2s", got, want)
	}
}

func TestPredictCameron(t *testing.T) {
	// 20:00 for 5 km -> 20 * 2 * f(5000)/f(10000) = 41.661 min, worked by hand
	// with f(x) = 13.49681 - 0.000030363x + 835.7114/x^0.7905.
	got := PredictCameron(5000, 20*time.Minute, 10000)
	want := 41*time.Minute + 40*time.Second

	if !within(got, want, 2*time.Second) {
		t.Errorf("PredictCameron(5 km in 20:00 -> 10 km) = %v, want %v ±2s", got, want)
	}
}

func TestPredictorsReturnTheKnownTimeForTheKnownDistance(t *testing.T) {
	known := 45*time.Minute + 30*time.Second

	for name, predict := range map[string]func(int, time.Duration, int) time.Duration{
		"riegel":  PredictRiegel,
		"cameron": PredictCameron,
	} {
		if got := predict(10000, known, 10000); !within(got, known, time.Second) {
			t.Errorf("%s: same distance = %v, want %v", name, got, known)
		}
	}
}

// Both models encode fatigue: doubling the distance more than doubles the time.
func TestPredictorsSlowDownOverLongerDistances(t *testing.T) {
	known := 20 * time.Minute

	for name, predict := range map[string]func(int, time.Duration, int) time.Duration{
		"riegel":  PredictRiegel,
		"cameron": PredictCameron,
	} {
		if got := predict(5000, known, 10000); got <= 2*known {
			t.Errorf("%s: 10 km predicted in %v, want more than %v", name, got, 2*known)
		}
	}
}

func TestPredictorsRejectNonsense(t *testing.T) {
	for name, predict := range map[string]func(int, time.Duration, int) time.Duration{
		"riegel":  PredictRiegel,
		"cameron": PredictCameron,
	} {
		if got := predict(0, 20*time.Minute, 10000); got != 0 {
			t.Errorf("%s: zero known distance = %v, want 0", name, got)
		}
		if got := predict(5000, 0, 10000); got != 0 {
			t.Errorf("%s: zero known time = %v, want 0", name, got)
		}
		if got := predict(5000, 20*time.Minute, 0); got != 0 {
			t.Errorf("%s: zero target distance = %v, want 0", name, got)
		}
	}
}
