package calculator

import "math"

// Pace The method returns the pace on the kilometer by distance and full time.
// The method accept parameters distance and seconds.
//
// A dist parameter is a number of meters (distance).
// A time parameter is a number of seconds on the race.
func Pace(dist int, time int) int {
	/*
		0. 10000/1000 = 10
		1. 2646/10=264.6
		2. 264.6/60 = 4.41
		3. 0.41 * 60 = 24.6 =>
		4. 4 + 24.6 = 4m 24.6s
	*/
	// 0
	tripDistance := float64(dist) / 1000
	// 1
	secondsOnLap := float64(time) / tripDistance
	// 2
	minutestValue := secondsOnLap / 60
	// 3
	minutes := int(minutestValue)
	seconds := int(math.Round((minutestValue - float64(int64(minutestValue))) * 60))
	// 4
	if minutes > 0 {
		return (minutes * 60) + seconds
	} else {
		return seconds
	}
}

// Time The method returns the time of race by distance and average pace on the kilometer.
// A dist parameter is a number of meters.
// A pace parameter it is number of seconds on the one kilometer.
func Time(dist int, pace int) int {
	/*
		1. 21097/1000 = 21,097
		2. 280*21.097 = 5907.16
		3. if 5907.16 >= 3600 -> 5907.16 /60 = 98.4526 (minutes),
	*/

	// 1
	raceDistance := float64(dist) / 1000
	// 2
	totalSeconds := float64(pace) * raceDistance

	return int(totalSeconds)
}
