package calculator

import (
	"testing"
	"time"
)

func TestEvenSplitsEveryKilometer(t *testing.T) {
	got := EvenSplits(10000, 5*time.Minute, 1000)

	if len(got) != 10 {
		t.Fatalf("len = %d, want 10", len(got))
	}

	for i, point := range got {
		wantDistance := (i + 1) * 1000
		wantElapsed := time.Duration(i+1) * 5 * time.Minute

		if point.Distance != wantDistance || point.Elapsed != wantElapsed {
			t.Errorf("split %d = {%d m, %v}, want {%d m, %v}",
				i, point.Distance, point.Elapsed, wantDistance, wantElapsed)
		}
	}
}

func TestEvenSplitsEndExactlyAtTheFinish(t *testing.T) {
	got := EvenSplits(21097, 4*time.Minute+30*time.Second, 5000)

	wantDistances := []int{5000, 10000, 15000, 20000, 21097}
	if len(got) != len(wantDistances) {
		t.Fatalf("len = %d, want %d", len(got), len(wantDistances))
	}

	for i, want := range wantDistances {
		if got[i].Distance != want {
			t.Errorf("split %d distance = %d, want %d", i, got[i].Distance, want)
		}
	}

	if got, want := got[3].Elapsed, 90*time.Minute; got != want {
		t.Errorf("elapsed at 20 km = %v, want %v", got, want)
	}

	// 270 s/km over 21.097 km is 5696.19 s, reported to the whole second.
	if got, want := got[4].Elapsed, time.Hour+34*time.Minute+56*time.Second; got != want {
		t.Errorf("elapsed at the finish = %v, want %v", got, want)
	}
}

func TestEvenSplitsRejectsNonsense(t *testing.T) {
	tests := []struct {
		name string
		dist int
		pace time.Duration
		step int
	}{
		{name: "zero distance", dist: 0, pace: 5 * time.Minute, step: 1000},
		{name: "zero step", dist: 10000, pace: 5 * time.Minute, step: 0},
		{name: "zero pace", dist: 10000, pace: 0, step: 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EvenSplits(tt.dist, tt.pace, tt.step); got != nil {
				t.Errorf("EvenSplits(%d, %v, %d) = %v, want nil", tt.dist, tt.pace, tt.step, got)
			}
		})
	}
}
