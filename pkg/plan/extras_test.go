package plan

import (
	"reflect"
	"testing"
	"time"
)

// Splits run at an even pace and end exactly on the finish, whatever the
// distance's remainder.
func TestSplits(t *testing.T) {
	half := Plan{Distance: 21097, Time: hms(1, 38, 48)}
	splits := half.Splits(5000)

	want := []Split{
		{5000, time.Duration(int64(half.Time) * 5000 / 21097)},
		{10000, time.Duration(int64(half.Time) * 10000 / 21097)},
		{15000, time.Duration(int64(half.Time) * 15000 / 21097)},
		{20000, time.Duration(int64(half.Time) * 20000 / 21097)},
		{21097, half.Time},
	}
	if !reflect.DeepEqual(splits, want) {
		t.Errorf("Splits(5000) = %v, want %v", splits, want)
	}

	if got := (Plan{Distance: 10000, Time: hms(0, 50, 0)}).Splits(5000); len(got) != 2 || got[1] != (Split{10000, hms(0, 50, 0)}) {
		t.Errorf("a distance that divides evenly ends on one finish row, got %v", got)
	}
}

// The step keeps a table readable: every kilometre while that fits, then
// round steps.
func TestSplitStep(t *testing.T) {
	tests := []struct {
		distance, maxRows, want int
	}{
		{5000, 25, 1000},
		{21097, 25, 1000},
		{30000, 25, 5000},
		{42195, 25, 5000},
		{100000, 25, 5000},
		{160934, 25, 10000},
		{500000, 25, 20000},
		{42195, 9, 5000},
	}
	for _, tt := range tests {
		if got := SplitStep(tt.distance, tt.maxRows); got != tt.want {
			t.Errorf("SplitStep(%d, %d) = %d, want %d", tt.distance, tt.maxRows, got, tt.want)
		}
	}
}

// Words the parser cannot place are reported, so a reply can quote them;
// fillers and units are not.
func TestUnrecognized(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		{"notapace 21095", []string{"notapace"}},
		{"5k abc 20:00 xyz", []string{"abc", "xyz"}},
		{"марафон за 3:30", nil},
		{"10k 4:50 per km", nil},
		{"half in 1:45", nil},
		{"10 км темп 5:00", nil},
	}
	for _, tt := range tests {
		if got := Unrecognized(tt.text); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Unrecognized(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}
