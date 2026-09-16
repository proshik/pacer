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

// ParseAs reads a request whose clock means what kind says; words the parser
// does not know, such as "за" or "in", are skipped.
func ParseAs(text string, kind Kind) (Plan, error) {
	tokens := strings.Fields(strings.ToLower(text))

	var (
		distances []int
		clocks    []clock
		paceSaid  = kind == Pace
	)

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]

		for _, marker := range []string{"/km", "/км"} {
			if trimmed, found := strings.CutSuffix(token, marker); found {
				token, paceSaid = trimmed, true
			}
		}
		if token == "pace" || token == "темп" {
			paceSaid = true
			continue
		}
		if token == "" {
			continue
		}

		// A number and its unit written apart: "21,1 км", "5 mi".
		if i+1 < len(tokens) && isUnit(tokens[i+1]) {
			if meters, ok := ParseDistance(token + tokens[i+1]); ok {
				distances = append(distances, meters)
				i++
				continue
			}
		}

		if c, ok := parseClock(token); ok {
			clocks = append(clocks, c)
			continue
		}

		if meters, ok := ParseDistance(token); ok {
			distances = append(distances, meters)
		}
	}

	switch {
	case len(distances) == 0:
		return Plan{}, ErrNoDistance
	case len(clocks) == 0:
		return Plan{}, ErrNoTime
	case len(distances) > 1 || len(clocks) > 1:
		return Plan{}, ErrAmbiguous
	}

	if paceSaid {
		kind = Pace
	}

	return resolve(distances[0], clocks[0], kind)
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
