package main

import (
	"fmt"
	"gorun/pkg/calculator"
	"strconv"
	"syscall/js"
	"time"
)

func paceWrapper(c *calculator.Service) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		//if len(args) != 1 {
		//	result := map[string]interface{}{
		//		"error": "Invalid no of arguments passed",
		//	}
		//	return result
		//}
		//inputJSON := args[0].String()

		jsDoc := js.Global().Get("document")
		if !jsDoc.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get document object",
			}
			return result
		}

		distInput := jsDoc.Call("getElementById", "distInput")
		if !distInput.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get distInput element",
			}
			return result
		}

		timeHourInput := jsDoc.Call("getElementById", "timeHourInput")
		if !timeHourInput.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get timeHourInput element",
			}
			return result
		}

		timeMinuteInput := jsDoc.Call("getElementById", "timeMinuteInput")
		if !timeHourInput.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get timeMinuteInput element",
			}
			return result
		}

		timeSecondInput := jsDoc.Call("getElementById", "timeSecondInput")
		if !timeHourInput.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get timeSecondInput element",
			}
			return result
		}

		paceMinuteInput := jsDoc.Call("getElementById", "paceMinuteInput")
		if !timeHourInput.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get paceMinuteInput element",
			}
			return result
		}

		paceSecondInput := jsDoc.Call("getElementById", "paceSecondInput")
		if !timeHourInput.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get paceSecondInput element",
			}
			return result
		}

		dist, err := strconv.Atoi(distInput.Get("value").String())
		if err != nil {
			result := map[string]interface{}{
				"error": "Distance value is not a numeric: " + distInput.String(),
			}
			return result
		}

		hours := timeHourInput.Get("value").String()
		minutes := timeMinuteInput.Get("value").String()
		seconds := timeSecondInput.Get("value").String()

		timeString := fmt.Sprintf("%sh%sm%ss", hours, minutes, seconds)

		timeValue, err := time.ParseDuration(timeString)
		if err != nil {
			result := map[string]interface{}{
				"error": "Can`t parse time to duration: " + timeString,
			}
			return result
		}

		paceResult := c.Pace(dist, timeValue)

		paceMinuteInput.Set("value", strconv.Itoa(int(paceResult.Minutes())%60))
		paceSecondInput.Set("value", strconv.Itoa(int(paceResult.Seconds())%60))

		return "pace result: " + paceResult.String()
	})
}

func timeWrapper(c *calculator.Service) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		jsDoc := js.Global().Get("document")
		if !jsDoc.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get document object",
			}
			return result
		}

		distInput := jsDoc.Call("getElementById", "distInput")
		if !distInput.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get distInput element",
			}
			return result
		}

		timeHourInput := jsDoc.Call("getElementById", "timeHourInput")
		if !timeHourInput.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get timeHourInput element",
			}
			return result
		}

		timeMinuteInput := jsDoc.Call("getElementById", "timeMinuteInput")
		if !timeHourInput.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get timeMinuteInput element",
			}
			return result
		}

		timeSecondInput := jsDoc.Call("getElementById", "timeSecondInput")
		if !timeHourInput.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get timeSecondInput element",
			}
			return result
		}

		paceMinuteInput := jsDoc.Call("getElementById", "paceMinuteInput")
		if !timeHourInput.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get paceMinuteInput element",
			}
			return result
		}

		paceSecondInput := jsDoc.Call("getElementById", "paceSecondInput")
		if !timeHourInput.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get paceSecondInput element",
			}
			return result
		}

		dist, err := strconv.Atoi(distInput.Get("value").String())
		if err != nil {
			result := map[string]interface{}{
				"error": "Distance value is not a numeric: " + distInput.String(),
			}
			return result
		}

		minutes := paceMinuteInput.Get("value").String()
		seconds := paceSecondInput.Get("value").String()

		paceString := fmt.Sprintf("%sm%ss", minutes, seconds)

		paceValue, err := time.ParseDuration(paceString)
		if err != nil {
			result := map[string]interface{}{
				"error": "Can`t parse pace to duration: " + paceString,
			}
			return result
		}

		timeResult := c.Time(dist, paceValue)

		timeMinuteInput.Set("value", strconv.Itoa(int(timeResult.Hours())%60))
		timeMinuteInput.Set("value", strconv.Itoa(int(timeResult.Minutes())%60))
		timeSecondInput.Set("value", strconv.Itoa(int(timeResult.Seconds())%60))

		return "time result: " + timeResult.String()
	})
}

func main() {
	fmt.Println("Go Web Assembly")

	c := calculator.NewService()

	js.Global().Set("calcPace", paceWrapper(c))
	js.Global().Set("calcTime", timeWrapper(c))

	<-make(chan bool)
}
