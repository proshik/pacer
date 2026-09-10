package calculator

import (
	"math"
	"testing"
	"time"
)

// Equivalent performances from Daniels' published VDOT table
// (https://milo.app/tools/vdot-chart). The table is generated from the same
// Daniels-Gilbert equations, so the formula should land on it closely.
var publishedEquivalents = []struct {
	name string
	vdot float64
	dist int
	race time.Duration
}{
	{name: "5K at VDOT 40", vdot: 40, dist: 5000, race: 24*time.Minute + 8*time.Second},
	{name: "marathon at VDOT 40", vdot: 40, dist: 42195, race: 3*time.Hour + 49*time.Minute + 45*time.Second},
	{name: "10K at VDOT 45", vdot: 45, dist: 10000, race: 45*time.Minute + 16*time.Second},
	{name: "5K at VDOT 50", vdot: 50, dist: 5000, race: 19*time.Minute + 57*time.Second},
	{name: "half at VDOT 50", vdot: 50, dist: 21097, race: time.Hour + 31*time.Minute + 35*time.Second},
	{name: "marathon at VDOT 50", vdot: 50, dist: 42195, race: 3*time.Hour + 10*time.Minute + 49*time.Second},
	{name: "5K at VDOT 60", vdot: 60, dist: 5000, race: 17*time.Minute + 1*time.Second},
	{name: "marathon at VDOT 60", vdot: 60, dist: 42195, race: 2*time.Hour + 43*time.Minute + 25*time.Second},
}

func TestVDOTMatchesPublishedTable(t *testing.T) {
	for _, tt := range publishedEquivalents {
		t.Run(tt.name, func(t *testing.T) {
			if got := VDOT(tt.dist, tt.race); math.Abs(got-tt.vdot) > 0.3 {
				t.Errorf("VDOT(%d m, %v) = %.2f, want %.0f ±0.3", tt.dist, tt.race, got, tt.vdot)
			}
		})
	}
}

func TestRaceTimeForVDOTMatchesPublishedTable(t *testing.T) {
	for _, tt := range publishedEquivalents {
		t.Run(tt.name, func(t *testing.T) {
			// Table entries are rounded, so each sits a hair off the exact
			// integer VDOT; over a marathon that is worth several seconds.
			if got := RaceTimeForVDOT(tt.vdot, tt.dist); !within(got, tt.race, 15*time.Second) {
				t.Errorf("RaceTimeForVDOT(%.0f, %d m) = %v, want %v ±15s", tt.vdot, tt.dist, got, tt.race)
			}
		})
	}
}

func TestRaceTimeForVDOTInvertsVDOT(t *testing.T) {
	for _, tt := range publishedEquivalents {
		t.Run(tt.name, func(t *testing.T) {
			vdot := VDOT(tt.dist, tt.race)

			if got := RaceTimeForVDOT(vdot, tt.dist); !within(got, tt.race, time.Second) {
				t.Errorf("round trip for %v over %d m = %v", tt.race, tt.dist, got)
			}
		})
	}
}

func TestTrainingPacesForVDOT50(t *testing.T) {
	paces := TrainingPacesForVDOT(50)

	// Published per-kilometer paces for VDOT 50: marathon 4'31, threshold 4'15.
	if want := 4*time.Minute + 31*time.Second; !within(paces.Marathon, want, 3*time.Second) {
		t.Errorf("marathon pace = %v, want %v ±3s", paces.Marathon, want)
	}
	if want := 4*time.Minute + 15*time.Second; !within(paces.Threshold, want, 3*time.Second) {
		t.Errorf("threshold pace = %v, want %v ±3s", paces.Threshold, want)
	}

	// Zones must get strictly faster as intensity rises.
	ordered := []struct {
		name string
		pace time.Duration
	}{
		{name: "easy, slow end", pace: paces.EasySlow},
		{name: "easy, fast end", pace: paces.EasyFast},
		{name: "marathon", pace: paces.Marathon},
		{name: "threshold", pace: paces.Threshold},
		{name: "interval", pace: paces.Interval},
	}
	for i := 1; i < len(ordered); i++ {
		if ordered[i].pace >= ordered[i-1].pace {
			t.Errorf("%s pace %v is not faster than %s pace %v",
				ordered[i].name, ordered[i].pace, ordered[i-1].name, ordered[i-1].pace)
		}
	}
}

func TestVDOTRejectsNonsense(t *testing.T) {
	if got := VDOT(0, 20*time.Minute); got != 0 {
		t.Errorf("VDOT with zero distance = %v, want 0", got)
	}
	if got := VDOT(5000, 0); got != 0 {
		t.Errorf("VDOT with zero time = %v, want 0", got)
	}
	if got := RaceTimeForVDOT(0, 5000); got != 0 {
		t.Errorf("RaceTimeForVDOT with zero VDOT = %v, want 0", got)
	}
}
