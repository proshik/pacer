package wasmcalc_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"gorun/pkg/calculator"
	"gorun/pkg/wasmcalc"
)

func newEngine(t *testing.T) *wasmcalc.Engine {
	t.Helper()

	engine, err := wasmcalc.New(context.Background())
	if err != nil {
		t.Fatalf("create wasm engine: %v", err)
	}

	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Errorf("close wasm engine: %v", err)
		}
	})

	return engine
}

// The whole point of shipping one artifact is that the browser, the bot and the
// API cannot drift apart. This asserts the wasm build and the native build of
// pkg/calculator agree, which is the property that guarantees it.
func TestWasmEngineMatchesNativeCalculator(t *testing.T) {
	engine := newEngine(t)
	native := calculator.NewService()

	cases := []struct {
		name string
		dist int
		pace time.Duration
		race time.Duration
	}{
		{name: "5k", dist: 5000, pace: 4*time.Minute + 9*time.Second, race: 20*time.Minute + 45*time.Second},
		{name: "10k", dist: 10000, pace: 5 * time.Minute, race: 50 * time.Minute},
		{name: "half", dist: 21097, pace: 4*time.Minute + 41*time.Second, race: time.Hour + 38*time.Minute + 48*time.Second},
		{name: "marathon", dist: 42195, pace: 5*time.Minute + 19*time.Second, race: 3*time.Hour + 44*time.Minute + 20*time.Second},
		{name: "100 km ultra", dist: 100000, pace: 10 * time.Minute, race: 16*time.Hour + 40*time.Minute},
		{name: "one meter", dist: 1, pace: 5 * time.Minute, race: time.Second},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := engine.Time(tt.dist, tt.pace), native.Time(tt.dist, tt.pace); got != want {
				t.Errorf("Time(%d, %v) = %v, native = %v", tt.dist, tt.pace, got, want)
			}

			if got, want := engine.Pace(tt.dist, tt.race), native.Pace(tt.dist, tt.race); got != want {
				t.Errorf("Pace(%d, %v) = %v, native = %v", tt.dist, tt.race, got, want)
			}
		})
	}
}

// A single wasm instance is shared by every HTTP request, so calls must be
// serialized. Run under -race to make a missing lock visible.
func TestWasmEngineIsSafeForConcurrentUse(t *testing.T) {
	engine := newEngine(t)
	native := calculator.NewService()

	const goroutines = 16
	const iterations = 25

	var wg sync.WaitGroup
	errs := make(chan string, goroutines*iterations)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()

			dist := 1000 * (worker + 1)
			for j := 0; j < iterations; j++ {
				if got, want := engine.Time(dist, 5*time.Minute), native.Time(dist, 5*time.Minute); got != want {
					errs <- "Time mismatch under concurrency"
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for message := range errs {
		t.Fatal(message)
	}
}
