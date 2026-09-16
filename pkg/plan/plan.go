// Package plan reads a running plan the way a runner writes it: a distance
// and either a finish time or a pace, in free form and in either language.
package plan

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"gorun/pkg/calculator"
)

var (
	ErrNoDistance  = errors.New("no distance")
	ErrNoTime      = errors.New("no time or pace")
	ErrAmbiguous   = errors.New("more than one distance or time")
	ErrImplausible = errors.New("no plausible pace")
)

// Kind says what the clock in a request means.
type Kind int

const (
	// Auto decides from the numbers: a finish time if that gives a running
	// pace, otherwise a pace.
	Auto Kind = iota
	FinishTime
	Pace
)

// A pace outside this range is not running: the fast end is quicker than any
// road record per kilometre, the slow end is a walk with breaks.
const (
	fastestPace = 90 * time.Second
	slowestPace = 20 * time.Minute
)

// Plan is a distance and a finish time; the pace follows from them.
type Plan struct {
	Distance int // meters
	Time     time.Duration
}

// Pace is the time per kilometre, computed by the same calculator as
// everywhere else.
func (p Plan) Pace() time.Duration {
	return calculator.NewService().Pace(p.Distance, p.Time)
}

var namedDistances = map[string]int{
	"half":          21097,
	"halfmarathon":  21097,
	"half-marathon": 21097,
	"полумарафон":   21097,
	"marathon":      42195,
	"марафон":       42195,
}

// Longer suffixes come first, so "km" is not read as "k" plus a stray "m".
var units = []struct {
	suffix string
	meters float64
}{
	{"миль", 1609.344},
	{"мили", 1609.344},
	{"миля", 1609.344},
	{"km", 1000},
	{"км", 1000},
	{"mi", 1609.344},
	{"k", 1000},
	{"к", 1000},
	{"m", 1},
	{"м", 1},
}

func isUnit(token string) bool {
	for _, unit := range units {
		if token == unit.suffix {
			return true
		}
	}

	return false
}

// ParseDistance reads one distance: 21097, 21.1, 21,1км, 800m, 10k, 5mi,
// half, полумарафон. A bare whole number from 400 up is metres (a track
// distance or a race given in metres); any other bare number is kilometres.
func ParseDistance(text string) (int, bool) {
	s := strings.ToLower(strings.TrimSpace(text))
	if s == "" {
		return 0, false
	}

	if meters, ok := namedDistances[s]; ok {
		return meters, true
	}

	for _, unit := range units {
		if number, found := strings.CutSuffix(s, unit.suffix); found {
			value, ok := parseNumber(number)
			if !ok {
				return 0, false
			}

			meters := int(math.Round(value * unit.meters))
			return meters, meters > 0
		}
	}

	value, ok := parseNumber(s)
	if !ok {
		return 0, false
	}

	if !strings.ContainsAny(s, ".,") && value >= 400 {
		return int(value), true
	}

	meters := int(math.Round(value * 1000))
	return meters, meters > 0
}

// parseNumber accepts digits with at most one decimal point or comma.
func parseNumber(s string) (float64, bool) {
	s = strings.Replace(s, ",", ".", 1)
	if s == "" || strings.Trim(s, "0123456789.") != "" || strings.Count(s, ".") > 1 {
		return 0, false
	}

	value, err := strconv.ParseFloat(s, 64)
	if err != nil || value <= 0 {
		return 0, false
	}

	return value, true
}

// clock is a parsed time value. A two-part clock such as 3:30 also carries
// its hours-and-minutes reading, which a marathon needs and a 5 km does not.
type clock struct {
	value time.Duration
	hours time.Duration
}

// ParseClock reads one time value: 1:38:48, 38:48, 4:50, or Go's 1h38m48s.
// A bare number is never a clock, so it stays free to be a distance.
func ParseClock(text string) (time.Duration, bool) {
	c, ok := parseClock(text)
	return c.value, ok
}

func parseClock(text string) (clock, bool) {
	s := strings.TrimSpace(text)
	if s == "" {
		return clock{}, false
	}

	if strings.Contains(s, ":") {
		return parseColonClock(s)
	}

	if !strings.ContainsAny(s, "hms") {
		return clock{}, false
	}

	value, err := time.ParseDuration(s)
	if err != nil || value <= 0 {
		return clock{}, false
	}

	return clock{value: value}, true
}

func parseColonClock(s string) (clock, bool) {
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return clock{}, false
	}

	numbers := make([]int, len(parts))
	for i, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return clock{}, false
		}

		number, err := strconv.Atoi(part)
		if err != nil {
			return clock{}, false
		}
		numbers[i] = number
	}

	unit := func(n int, d time.Duration) time.Duration { return time.Duration(n) * d }

	if len(numbers) == 3 {
		hours, minutes, seconds := numbers[0], numbers[1], numbers[2]
		if minutes > 59 || seconds > 59 {
			return clock{}, false
		}

		value := unit(hours, time.Hour) + unit(minutes, time.Minute) + unit(seconds, time.Second)
		return clock{value: value}, value > 0
	}

	first, second := numbers[0], numbers[1]
	if second > 59 {
		return clock{}, false
	}

	value := unit(first, time.Minute) + unit(second, time.Second)
	return clock{value: value, hours: unit(first, time.Hour) + unit(second, time.Minute)}, value > 0
}

// Parse reads a free-form request in any order: "марафон 3:30", "10k 4:50",
// "21,1 км 1:38:48".
func Parse(text string) (Plan, error) {
	return ParseAs(text, Auto)
}

// ParseAs reads a request whose clock means what kind says; filler words
// such as "за" or "in" are skipped.
func ParseAs(text string, kind Kind) (Plan, error) {
	s := scanText(text)

	switch {
	case len(s.distances) == 0:
		return Plan{}, ErrNoDistance
	case len(s.clocks) == 0:
		return Plan{}, ErrNoTime
	case len(s.distances) > 1 || len(s.clocks) > 1:
		return Plan{}, ErrAmbiguous
	}

	if s.paceSaid {
		kind = Pace
	}

	return resolve(s.distances[0], s.clocks[0], kind)
}

// Unrecognized lists the words of a request that are neither a distance, a
// clock, a unit nor a filler, so a reply can quote what it did not understand.
func Unrecognized(text string) []string {
	return scanText(text).unknown
}

// fillers are the words a request carries around its numbers: "марафон за
// 3:30", "half in 1:45", "4:50 per km".
var fillers = map[string]bool{
	"in": true, "at": true, "for": true, "per": true, "a": true, "the": true,
	"за": true, "на": true, "в": true, "по": true, "это": true,
}

type scan struct {
	distances []int
	clocks    []clock
	paceSaid  bool
	unknown   []string
}

func scanText(text string) scan {
	var s scan
	tokens := strings.Fields(strings.ToLower(text))

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]

		for _, marker := range []string{"/km", "/км"} {
			if trimmed, found := strings.CutSuffix(token, marker); found {
				token, s.paceSaid = trimmed, true
			}
		}

		switch {
		case token == "":
			continue
		case token == "pace" || token == "темп":
			s.paceSaid = true
			continue
		case fillers[token] || isUnit(token):
			continue
		}

		// A number and its unit written apart: "21,1 км", "5 mi".
		if i+1 < len(tokens) && isUnit(tokens[i+1]) {
			if meters, ok := ParseDistance(token + tokens[i+1]); ok {
				s.distances = append(s.distances, meters)
				i++
				continue
			}
		}

		if c, ok := parseClock(token); ok {
			s.clocks = append(s.clocks, c)
			continue
		}

		if meters, ok := ParseDistance(token); ok {
			s.distances = append(s.distances, meters)
			continue
		}

		s.unknown = append(s.unknown, tokens[i])
	}

	return s
}

func resolve(distance int, c clock, kind Kind) (Plan, error) {
	if kind != Pace {
		for _, total := range []time.Duration{c.value, c.hours} {
			candidate := Plan{Distance: distance, Time: total}
			if total > 0 && plausible(candidate.Pace()) {
				return candidate, nil
			}
		}

		if kind == FinishTime {
			return Plan{}, ErrImplausible
		}
	}

	if !plausible(c.value) {
		return Plan{}, ErrImplausible
	}

	return Plan{Distance: distance, Time: calculator.NewService().Time(distance, c.value)}, nil
}

func plausible(pace time.Duration) bool {
	return pace >= fastestPace && pace <= slowestPace
}

// Split is the time from the start at a mark, running at an even pace.
type Split struct {
	Distance int // meters
	Elapsed  time.Duration
}

// Splits marks the plan every step metres at an even pace. The last mark is
// the finish, so a table always ends on the planned time.
func (p Plan) Splits(step int) []Split {
	if step <= 0 || p.Distance <= 0 {
		return nil
	}

	var splits []Split
	for mark := step; mark < p.Distance; mark += step {
		elapsed := time.Duration(int64(p.Time) * int64(mark) / int64(p.Distance))
		splits = append(splits, Split{Distance: mark, Elapsed: elapsed})
	}

	return append(splits, Split{Distance: p.Distance, Elapsed: p.Time})
}

// SplitStep picks the finest round step that keeps a split table within
// maxRows rows: every kilometre while that fits, then 5, 10, 20, 50 or 100 km.
func SplitStep(distance, maxRows int) int {
	steps := []int{1000, 5000, 10000, 20000, 50000, 100000}
	for _, step := range steps {
		rows := (distance + step - 1) / step // the marks and the finish
		if rows <= maxRows {
			return step
		}
	}

	return steps[len(steps)-1]
}
