package main

import (
	"os"
	"path/filepath"
	"testing"
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
