package calculator

import (
	"math"
	"time"
)

// riegelExponent is Peter Riegel's fatigue factor (American Scientist, 1981),
// fitted for efforts lasting roughly 3.5 to 230 minutes.
const riegelExponent = 1.06

// PredictRiegel estimates the time over targetDist from a known result,
// T2 = T1 * (D2/D1)^1.06. Distances are meters.
func PredictRiegel(knownDist int, knownTime time.Duration, targetDist int) time.Duration {
	if knownDist <= 0 || knownTime <= 0 || targetDist <= 0 {
		return 0
	}

	ratio := float64(targetDist) / float64(knownDist)

	return roundToSecond(float64(knownTime) * math.Pow(ratio, riegelExponent))
}

// PredictCameron estimates the time over targetDist with Dave Cameron's model,
// T2 = T1 * (D2/D1) * f(D1)/f(D2), intended for 800 m to the marathon.
// Distances are meters.
func PredictCameron(knownDist int, knownTime time.Duration, targetDist int) time.Duration {
	if knownDist <= 0 || knownTime <= 0 || targetDist <= 0 {
		return 0
	}

	known := float64(knownDist)
	target := float64(targetDist)

	return roundToSecond(float64(knownTime) * (target / known) * cameronFactor(known) / cameronFactor(target))
}

// cameronFactor is f(x) = 13.49681 - 0.000030363x + 835.7114/x^0.7905, x in meters.
func cameronFactor(meters float64) float64 {
	return 13.49681 - 0.000030363*meters + 835.7114/math.Pow(meters, 0.7905)
}

func roundToSecond(nanoseconds float64) time.Duration {
	return time.Duration(nanoseconds).Round(time.Second)
}
