package calculator

import (
	"math"
	"time"
)

const marathonMeters = 42195

// VDOT estimates Jack Daniels' VDOT from a race result with the Daniels-Gilbert
// equations: the oxygen cost of the race speed divided by the fraction of
// VO2max that can be sustained for the race duration.
func VDOT(dist int, raceTime time.Duration) float64 {
	if dist <= 0 || raceTime <= 0 {
		return 0
	}

	minutes := raceTime.Minutes()
	velocity := float64(dist) / minutes // meters per minute

	return oxygenCost(velocity) / sustainableFraction(minutes)
}

// RaceTimeForVDOT finds the time over dist that corresponds to vdot. VDOT
// falls as the race time grows, so bisection over one minute to a day converges.
func RaceTimeForVDOT(vdot float64, dist int) time.Duration {
	if vdot <= 0 || dist <= 0 {
		return 0
	}

	low, high := 1.0, 24*60.0 // minutes
	for range 100 {
		middle := (low + high) / 2
		if VDOT(dist, minutesToDuration(middle)) > vdot {
			low = middle // still faster than vdot allows
		} else {
			high = middle
		}
	}

	return minutesToDuration((low + high) / 2).Round(time.Second)
}

// TrainingPaces are Daniels' training intensities for a VDOT, as time per
// kilometer.
//
// Repetition pace is deliberately absent: the sources checked describe it only
// as "faster than interval", not as a fraction of VO2max, so any number here
// would be invented.
type TrainingPaces struct {
	EasySlow  time.Duration // 59% of VDOT
	EasyFast  time.Duration // 74% of VDOT
	Marathon  time.Duration // pace of the VDOT-equivalent marathon
	Threshold time.Duration // 88% of VDOT
	Interval  time.Duration // 98% of VDOT
}

// TrainingPacesForVDOT derives the training paces for vdot.
func TrainingPacesForVDOT(vdot float64) TrainingPaces {
	if vdot <= 0 {
		return TrainingPaces{}
	}

	return TrainingPaces{
		EasySlow:  paceAtFraction(vdot, 0.59),
		EasyFast:  paceAtFraction(vdot, 0.74),
		Marathon:  paceOver(RaceTimeForVDOT(vdot, marathonMeters), marathonMeters),
		Threshold: paceAtFraction(vdot, 0.88),
		Interval:  paceAtFraction(vdot, 0.98),
	}
}

// oxygenCost is VO2 in ml/kg/min for running at velocity meters per minute.
func oxygenCost(velocity float64) float64 {
	return -4.60 + 0.182258*velocity + 0.000104*velocity*velocity
}

// sustainableFraction is the share of VO2max that can be held for minutes.
func sustainableFraction(minutes float64) float64 {
	return 0.8 + 0.1894393*math.Exp(-0.012778*minutes) + 0.2989558*math.Exp(-0.1932605*minutes)
}

// velocityForOxygenCost inverts oxygenCost: the speed, in meters per minute,
// whose oxygen cost is vo2. It takes the positive root of the quadratic.
func velocityForOxygenCost(vo2 float64) float64 {
	const a, b = 0.000104, 0.182258
	c := -4.60 - vo2

	return (-b + math.Sqrt(b*b-4*a*c)) / (2 * a)
}

// paceAtFraction is the time per kilometer at the speed costing fraction*vdot.
func paceAtFraction(vdot float64, fraction float64) time.Duration {
	velocity := velocityForOxygenCost(vdot * fraction)

	return minutesToDuration(1000 / velocity).Round(time.Second)
}

func paceOver(raceTime time.Duration, dist int) time.Duration {
	return time.Duration(float64(raceTime) * 1000 / float64(dist)).Round(time.Second)
}

func minutesToDuration(minutes float64) time.Duration {
	return time.Duration(minutes * float64(time.Minute))
}
