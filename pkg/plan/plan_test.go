package plan

import (
	"errors"
	"testing"
	"time"
)

func TestParseDistance(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"21097", 21097},
		{"400", 400},
		{"21.1", 21100},
		{"21,1", 21100},
		{"42.195", 42195},
		{"10", 10000},
		{"100", 100000},
		{"21.1km", 21100},
		{"21,1км", 21100},
		{"800m", 800},
		{"800м", 800},
		{"10k", 10000},
		{"10К", 10000},
		{"5K", 5000},
		{"5mi", 8047},
		{"half", 21097},
		{"Полумарафон", 21097},
		{"marathon", 42195},
		{"марафон", 42195},
	}
	for _, tt := range tests {
		got, ok := ParseDistance(tt.text)
		if !ok || got != tt.want {
			t.Errorf("ParseDistance(%q) = %d, %v; want %d", tt.text, got, ok, tt.want)
		}
	}

	for _, text := range []string{"", "abc", "-5", "0", "1:30", "10kk", "4m50s"} {
		if got, ok := ParseDistance(text); ok {
			t.Errorf("ParseDistance(%q) = %d, want no distance", text, got)
		}
	}
}

func TestParseClock(t *testing.T) {
	tests := []struct {
		text string
		want time.Duration
	}{
		{"1:38:48", time.Hour + 38*time.Minute + 48*time.Second},
		{"38:48", 38*time.Minute + 48*time.Second},
		{"4:50", 4*time.Minute + 50*time.Second},
		{"1h38m48s", time.Hour + 38*time.Minute + 48*time.Second},
		{"4m50s", 4*time.Minute + 50*time.Second},
	}
	for _, tt := range tests {
		got, ok := ParseClock(tt.text)
		if !ok || got != tt.want {
			t.Errorf("ParseClock(%q) = %v, %v; want %v", tt.text, got, ok, tt.want)
		}
	}

	// A bare number is a distance, never a clock; a clock has to be one.
	for _, text := range []string{"", "10", "abc", "3:75", "1:60:00", "0:00", "1:2:3:4", "-4m"} {
		if got, ok := ParseClock(text); ok {
			t.Errorf("ParseClock(%q) = %v, want no clock", text, got)
		}
	}
}

func hms(h, m, s int) time.Duration {
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second
}

// A free-form request reads the way a runner means it: a two-part clock is
// minutes and seconds unless that gives an impossible pace, and a clock that
// cannot be a finish time for the distance is a pace.
func TestParse(t *testing.T) {
	tests := []struct {
		text string
		want Plan
	}{
		{"марафон 3:30", Plan{42195, hms(3, 30, 0)}},
		{"полумарафон 1:38:48", Plan{21097, hms(1, 38, 48)}},
		{"half in 1:45", Plan{21097, hms(1, 45, 0)}},
		{"5k 25:00", Plan{5000, hms(0, 25, 0)}},
		{"25:00 5k", Plan{5000, hms(0, 25, 0)}},
		{"21,1 км 1:38:48", Plan{21100, hms(1, 38, 48)}},
		{"100k 12:00", Plan{100000, hms(12, 0, 0)}},
		{"10k 4:50", Plan{10000, hms(0, 48, 20)}},
		{"10k 4:50/км", Plan{10000, hms(0, 48, 20)}},
		{"10 км темп 5:00", Plan{10000, hms(0, 50, 0)}},
		{"1 km 4:00", Plan{1000, hms(0, 4, 0)}},
		{"21097 1h38m48s", Plan{21097, hms(1, 38, 48)}},
	}
	for _, tt := range tests {
		got, err := Parse(tt.text)
		if err != nil || got != tt.want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", tt.text, got, err, tt.want)
		}
	}
}

func TestParseRefusesWhatIsNotAPlan(t *testing.T) {
	tests := []struct {
		text string
		want error
	}{
		{"", ErrNoDistance},
		{"привет", ErrNoDistance},
		{"3:30", ErrNoDistance},
		{"марафон", ErrNoTime},
		{"5k 10k 20:00", ErrAmbiguous},
		{"5k 20:00 21:00", ErrAmbiguous},
		{"10k 99:59:00", ErrImplausible},
	}
	for _, tt := range tests {
		if _, err := Parse(tt.text); !errors.Is(err, tt.want) {
			t.Errorf("Parse(%q) error = %v, want %v", tt.text, err, tt.want)
		}
	}
}

// A command says what its clock is, so the order of arguments and the old Go
// duration syntax both keep working.
func TestParseAs(t *testing.T) {
	tests := []struct {
		text string
		kind Kind
		want Plan
	}{
		{"4:50 10k", Pace, Plan{10000, hms(0, 48, 20)}},
		{"10k 4:50", Pace, Plan{10000, hms(0, 48, 20)}},
		{"4m50s 21095", Pace, Plan{21095, hms(1, 41, 57)}},
		{"21097 1h38m48s", FinishTime, Plan{21097, hms(1, 38, 48)}},
		{"марафон 3:30", FinishTime, Plan{42195, hms(3, 30, 0)}},
		{"5k 20:00", FinishTime, Plan{5000, hms(0, 20, 0)}},
	}
	for _, tt := range tests {
		got, err := ParseAs(tt.text, tt.kind)
		if err != nil || got != tt.want {
			t.Errorf("ParseAs(%q, %v) = %+v, %v; want %+v", tt.text, tt.kind, got, err, tt.want)
		}
	}
}

func TestPlanPace(t *testing.T) {
	if got, want := (Plan{42195, hms(3, 44, 20)}).Pace(), hms(0, 5, 19); got != want {
		t.Errorf("pace of a 3:44:20 marathon = %v, want %v", got, want)
	}
}
