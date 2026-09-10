package calculator

import "time"

// Split breaks a duration into whole hours, minutes and seconds.
//
// Hours are not wrapped: a 61 hour ultra reports 61 hours, not 1.
func Split(duration time.Duration) (hours int, minutes int, seconds int) {
	totalSeconds := int(duration.Seconds())

	return totalSeconds / 3600, (totalSeconds % 3600) / 60, totalSeconds % 60
}
