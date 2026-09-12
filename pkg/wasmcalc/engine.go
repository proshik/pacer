// Package wasmcalc runs the pace calculator from its compiled WebAssembly
// form instead of from linked Go code.
//
// The browser already executes the calculator as wasm. Executing the very same
// artifact on the server too means the formulas cannot drift between the bot,
// the HTTP API and the page: there is one compiled copy, not one per host.
// wazero is used because it is a pure Go runtime with no CGO, so cross
// compiling the server and building the Docker image stay unchanged.
package wasmcalc

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"gorun/pkg/calculator"
)

// calc.wasm is produced by ./build.sh from ./cmd/calcwasm and committed, so a
// clean checkout compiles without a wasm toolchain present.
//
//go:embed calc.wasm
var calcWasm []byte

// Engine is a calculator.Engine backed by the compiled wasm module.
type Engine struct {
	runtime wazero.Runtime
	module  api.Module

	// One module instance is shared by every caller and wasm calls against it
	// are not safe to run concurrently, so calls are serialized. The work is
	// arithmetic, so the lock is never held long.
	mu       sync.Mutex
	calcTime api.Function
	calcPace api.Function
}

var _ calculator.Engine = (*Engine)(nil)

// New loads the embedded module in WASI reactor mode.
func New(ctx context.Context) (*Engine, error) {
	runtime := wazero.NewRuntime(ctx)

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		_ = runtime.Close(ctx)
		return nil, fmt.Errorf("instantiate wasi preview1: %w", err)
	}

	// A reactor exports _initialize instead of _start: it sets the module up
	// and returns, leaving the exported functions callable afterwards.
	module, err := runtime.InstantiateWithConfig(
		ctx,
		calcWasm,
		wazero.NewModuleConfig().WithStartFunctions("_initialize"),
	)
	if err != nil {
		_ = runtime.Close(ctx)
		return nil, fmt.Errorf("instantiate calc module: %w", err)
	}

	engine := &Engine{
		runtime:  runtime,
		module:   module,
		calcTime: module.ExportedFunction("calc_time"),
		calcPace: module.ExportedFunction("calc_pace"),
	}

	if engine.calcTime == nil || engine.calcPace == nil {
		_ = runtime.Close(ctx)
		return nil, fmt.Errorf("calc module does not export calc_time and calc_pace")
	}

	return engine, nil
}

// Time returns the finish time for a distance in meters at the given pace.
func (e *Engine) Time(dist int, pace time.Duration) time.Duration {
	return e.callSeconds(e.calcTime, "calc_time", dist, int(pace.Seconds()))
}

// Pace returns the pace per kilometer for a distance in meters and a finish time.
func (e *Engine) Pace(dist int, timeValue time.Duration) time.Duration {
	return e.callSeconds(e.calcPace, "calc_pace", dist, int(timeValue.Seconds()))
}

// Close releases the runtime and the module instance.
func (e *Engine) Close(ctx context.Context) error {
	if err := e.runtime.Close(ctx); err != nil {
		return fmt.Errorf("close wasm runtime: %w", err)
	}

	return nil
}

// callSeconds crosses the wasm boundary. The exported ABI is flat int32, so
// both arguments are seconds or meters and the result is seconds.
//
// A failure here means the module itself is broken, which New would already
// have rejected; it is logged and reported as a zero duration rather than
// propagated, so the engine stays swappable with the native calculator.
func (e *Engine) callSeconds(fn api.Function, name string, first int, second int) time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()

	results, err := fn.Call(
		context.Background(),
		api.EncodeI32(int32(first)),
		api.EncodeI32(int32(second)),
	)
	if err != nil {
		slog.Error("wasm calculation failed", "err", err, "func", name)
		return 0
	}

	if len(results) != 1 {
		slog.Error("wasm calculation returned unexpected result count", "func", name, "count", len(results))
		return 0
	}

	return time.Duration(api.DecodeI32(results[0])) * time.Second
}
