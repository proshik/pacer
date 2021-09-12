package main

import (
	"encoding/json"
	"fmt"
	"gorun/pkg/calculator"
	"strconv"
	"syscall/js"
	"time"
	//"time"
)

func prettyJson(input string) (string, error) {
	var raw interface{}
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		return "", err
	}
	pretty, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return "", err
	}
	return string(pretty), nil
}

func jsonWrapper() js.Func {
	jsonFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) != 1 {
			result := map[string]interface{}{
				"error": "Invalid no of arguments passed",
			}
			return result
		}

		jsDoc := js.Global().Get("document")
		if !jsDoc.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get document object",
			}
			return result
		}

		jsonOutputTextArea := jsDoc.Call("getElementById", "jsonoutput")
		if !jsonOutputTextArea.Truthy() {
			result := map[string]interface{}{
				"error": "Unable to get output text area",
			}
			return result
		}

		inputJSON := args[0].String()
		fmt.Printf("input %s\n", inputJSON)

		pretty, err := prettyJson(inputJSON)
		if err != nil {
			errStr := fmt.Sprintf("unable to parse JSON. Error %s occurred\n", err)
			result := map[string]interface{}{
				"error": errStr,
			}
			return result
		}

		jsonOutputTextArea.Set("value", pretty)

		return nil
	})

	return jsonFunc
}

func paceWrapper(c *calculator.Service) js.Func {
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

		pace := c.Pace(dist, timeValue)

		paceMinuteInput.Set("value", strconv.Itoa(int(pace.Minutes())%60))
		paceSecondInput.Set("value", strconv.Itoa(int(pace.Seconds())%60))

		return pace.String()
	})
}

func timeWrapper(c *calculator.Service) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {

		return nil
	})
}

func main() {
	fmt.Println("Go Web Assembly")

	c := calculator.NewService()

	js.Global().Set("calcPace", paceWrapper(c))
	js.Global().Set("time", timeWrapper(c))

	js.Global().Set("formatJSON", jsonWrapper())

	<-make(chan bool)
}
