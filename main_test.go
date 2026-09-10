package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorun/pkg/wasmcalc"
)

func TestLoadDotEnvIfExists_MissingFile(t *testing.T) {
	t.Parallel()

	missingPath := filepath.Join(t.TempDir(), ".env")
	if err := loadDotEnvIfExists(missingPath); err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
}

func TestLoadDotEnvIfExists_LoadsValuesAndPreservesExistingEnv(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := "GORUN_TEST_FROM_FILE=from_file\nexport GORUN_TEST_KEEP=from_file\n"
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	oldFromFile, hadFromFile := os.LookupEnv("GORUN_TEST_FROM_FILE")
	oldKeep, hadKeep := os.LookupEnv("GORUN_TEST_KEEP")
	t.Cleanup(func() {
		if hadFromFile {
			_ = os.Setenv("GORUN_TEST_FROM_FILE", oldFromFile)
		} else {
			_ = os.Unsetenv("GORUN_TEST_FROM_FILE")
		}

		if hadKeep {
			_ = os.Setenv("GORUN_TEST_KEEP", oldKeep)
		} else {
			_ = os.Unsetenv("GORUN_TEST_KEEP")
		}
	})

	_ = os.Unsetenv("GORUN_TEST_FROM_FILE")
	if err := os.Setenv("GORUN_TEST_KEEP", "from_env"); err != nil {
		t.Fatalf("set env: %v", err)
	}

	if err := loadDotEnvIfExists(envPath); err != nil {
		t.Fatalf("load .env: %v", err)
	}

	if got := os.Getenv("GORUN_TEST_FROM_FILE"); got != "from_file" {
		t.Fatalf("GORUN_TEST_FROM_FILE mismatch: got %q", got)
	}

	if got := os.Getenv("GORUN_TEST_KEEP"); got != "from_env" {
		t.Fatalf("GORUN_TEST_KEEP mismatch: got %q", got)
	}
}

func TestNewCalculationEngineSelection(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantWasm  bool
		wantError bool
	}{
		{name: "empty defaults to native", value: "", wantWasm: false},
		{name: "native", value: "native", wantWasm: false},
		{name: "wasm", value: "wasm", wantWasm: true},
		{name: "case and spacing are forgiven", value: "  WASM ", wantWasm: true},
		{name: "unknown value is rejected", value: "quantum", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, closeEngine, err := newCalculationEngine(tt.value)

			if tt.wantError {
				if err == nil {
					_ = closeEngine(context.Background())
					t.Fatalf("expected an error for %q, got none", tt.value)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.value, err)
			}
			t.Cleanup(func() {
				if err := closeEngine(context.Background()); err != nil {
					t.Errorf("close engine: %v", err)
				}
			})

			_, isWasm := engine.(*wasmcalc.Engine)
			if isWasm != tt.wantWasm {
				t.Errorf("engine for %q is wasm=%v, want wasm=%v", tt.value, isWasm, tt.wantWasm)
			}

			// Whichever engine is selected, the arithmetic must agree.
			if got, want := engine.Time(10000, 5*time.Minute), 50*time.Minute; got != want {
				t.Errorf("Time(10000, 5m) = %v, want %v", got, want)
			}
		})
	}
}

func TestOpenHistory(t *testing.T) {
	store, err := openHistory("   ")
	if err != nil || store != nil {
		t.Fatalf("blank DB_PATH: store = %v, err = %v; want history disabled", store, err)
	}

	store, err = openHistory(filepath.Join(t.TempDir(), "pacer.db"))
	if err != nil || store == nil {
		t.Fatalf("temp DB_PATH: store = %v, err = %v; want an open store", store, err)
	}

	if err := store.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}
