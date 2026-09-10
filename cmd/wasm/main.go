//go:build js && wasm
// +build js,wasm

package main

import (
	"encoding/json"
	"fmt"
	"gorun/pkg/calculator"
	"strconv"
	"syscall/js"
	"time"
)

const getElementById = "getElementById"

type CalcResult struct {
	Hour   string `json:"hour,omitempty"`
	Minute string `json:"minute,omitempty"`
	Second string `json:"second,omitempty"`
	Error  string `json:"error,omitempty"`
}

var defaultError = `{ "error": "unexpected error" }`

// got input values from arguments of the function
func paceWrapper(c *calculator.Service) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) != 4 {
			return buildErrorResult("Invalid of arguments passed, should 4")
		}
		dist := args[0].Int()
		if dist <= 0 {
			return buildErrorResult("Distance value should be greater than zero")
		}

		hours := args[1].String()
		minutes := args[2].String()
		seconds := args[3].String()

		timeString := fmt.Sprintf("%sh%sm%ss", hours, minutes, seconds)

		timeValue, err := time.ParseDuration(timeString)
		if err != nil {
			return buildErrorResult("Can`t parse time to duration: " + timeString)
		}

		pace := c.Pace(dist, timeValue)

		return buildResult(pace)
	})
}

func buildResult(result time.Duration) string {
	hours, minutes, seconds := calculator.Split(result)

	b, err := json.Marshal(CalcResult{
		Hour:   strconv.Itoa(hours),
		Minute: strconv.Itoa(minutes),
		Second: strconv.Itoa(seconds),
	})
	if err != nil {
		return defaultError
	}

	return string(b)
}

// got values from value of document elements
func timeWrapper(c *calculator.Service) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		jsDoc := js.Global().Get("document")
		if !jsDoc.Truthy() {
			return buildErrorResult("Unable to get document object")
		}

		distInput := jsDoc.Call(getElementById, "distInput")
		if !distInput.Truthy() {
			return buildErrorResult("Unable to get distInput element")
		}

		timeHourInput := jsDoc.Call(getElementById, "timeHourInput")
		if !timeHourInput.Truthy() {
			return buildErrorResult("Unable to get timeHourInput element")
		}

		timeMinuteInput := jsDoc.Call(getElementById, "timeMinuteInput")
		if !timeMinuteInput.Truthy() {
			return buildErrorResult("Unable to get timeMinuteInput element")
		}

		timeSecondInput := jsDoc.Call(getElementById, "timeSecondInput")
		if !timeSecondInput.Truthy() {
			return buildErrorResult("Unable to get timeSecondInput element")
		}

		paceMinuteInput := jsDoc.Call(getElementById, "paceMinuteInput")
		if !paceMinuteInput.Truthy() {
			return buildErrorResult("Unable to get paceMinuteInput element")
		}

		paceSecondInput := jsDoc.Call(getElementById, "paceSecondInput")
		if !paceSecondInput.Truthy() {
			return buildErrorResult("Unable to get paceSecondInput element")
		}

		// convert distInput to int value
		dist, err := strconv.Atoi(distInput.Get("value").String())
		if err != nil {
			return buildErrorResult("Distance value is not a numeric: " + distInput.String())
		}
		if dist <= 0 {
			return buildErrorResult("Distance value should be greater than zero")
		}

		minutes := paceMinuteInput.Get("value").String()
		seconds := paceSecondInput.Get("value").String()
		if minutes == "" {
			minutes = "0"
		}
		if seconds == "" {
			seconds = "0"
		}

		paceString := fmt.Sprintf("%sm%ss", minutes, seconds)

		paceValue, err := time.ParseDuration(paceString)
		if err != nil {
			return buildErrorResult("Can`t parse pace to duration: " + paceString)
		}
		if paceValue <= 0 {
			return buildErrorResult("Pace should be greater than zero")
		}

		timeResult := c.Time(dist, paceValue)

		resultHours, resultMinutes, resultSeconds := calculator.Split(timeResult)
		timeHourInput.Set("value", strconv.Itoa(resultHours))
		timeMinuteInput.Set("value", strconv.Itoa(resultMinutes))
		timeSecondInput.Set("value", strconv.Itoa(resultSeconds))

		return buildResult(timeResult)
	})
}

func buildErrorResult(message string) string {
	b, err := json.Marshal(CalcResult{"", "", "", message})
	if err != nil {
		return defaultError
	}

	return string(b)
}

func main() {
	fmt.Println("Go Web Assembly")

	c := calculator.NewService()

	js.Global().Set("calcPace", paceWrapper(c))
	js.Global().Set("calcTime", timeWrapper(c))

	<-make(chan bool)
}
