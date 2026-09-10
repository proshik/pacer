package calculator

import "time"

// SplitPoint is the cumulative time at a distance marker of an even-paced run.
type SplitPoint struct {
	Distance int           // meters from the start
	Elapsed  time.Duration // cumulative time, rounded to the second
}

// EvenSplits returns a marker every step meters for a run held at pace (time
// per kilometer), plus one at the finish when dist is not a multiple of step.
func EvenSplits(dist int, pace time.Duration, step int) []SplitPoint {
	if dist <= 0 || pace <= 0 || step <= 0 {
		return nil
	}

	points := make([]SplitPoint, 0, dist/step+1)
	for marker := step; marker < dist; marker += step {
		points = append(points, splitAt(marker, pace))
	}

	return append(points, splitAt(dist, pace))
}

func splitAt(meters int, pace time.Duration) SplitPoint {
	elapsed := time.Duration(float64(pace) * float64(meters) / 1000)

	return SplitPoint{Distance: meters, Elapsed: elapsed.Round(time.Second)}
}
