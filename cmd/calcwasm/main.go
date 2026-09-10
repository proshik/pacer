// Command calcwasm exposes the pace calculator as a WASI reactor module.
//
// The same artifact this produces is executed on the server through wazero, so
// the bot, the HTTP API and any other host share one compiled copy of the
// formulas instead of one copy per language.
//
// Built with GOOS=wasip1 GOARCH=wasm and -buildmode=c-shared, which emits
// _initialize rather than _start and keeps the module callable after startup.
// The exported ABI is deliberately flat: go:wasmexport cannot pass pointers to
// structures containing pointers, so everything crosses the boundary as int32.
//go:build wasip1 && wasm

package main

import (
	"gorun/pkg/calculator"
)

//go:wasmexport calc_time
func calcTime(distMeters int32, paceSeconds int32) int32 {
	result := calculator.Time(int(distMeters), int(paceSeconds))

	return int32(result)
}

//go:wasmexport calc_pace
func calcPace(distMeters int32, raceSeconds int32) int32 {
	result := calculator.Pace(int(distMeters), int(raceSeconds))

	return int32(result)
}

// Kept so the package builds as a main package; a reactor never runs it.
func main() {}
