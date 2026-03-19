package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvSetsUnsetVariablesOnly(t *testing.T) {
	t.Setenv("FROM_PROCESS", "process-value")
	unsetEnvOnCleanup(t, "FROM_FILE")

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FROM_FILE=file-value\nFROM_PROCESS=file-override\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv returned error: %v", err)
	}

	if got := os.Getenv("FROM_FILE"); got != "file-value" {
		t.Fatalf("FROM_FILE = %q, want file-value", got)
	}
	if got := os.Getenv("FROM_PROCESS"); got != "process-value" {
		t.Fatalf("FROM_PROCESS = %q, want process-value", got)
	}
}

func TestLoadDotEnvSupportsExportAndQuotes(t *testing.T) {
	unsetEnvOnCleanup(t, "LOG_LEVEL", "SOURCE_NAME", "RECEIVER_URL")

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "export LOG_LEVEL=debug\nSOURCE_NAME='raspotify pi'\nRECEIVER_URL=\"http://receiver.local\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv returned error: %v", err)
	}

	if got := os.Getenv("LOG_LEVEL"); got != "debug" {
		t.Fatalf("LOG_LEVEL = %q, want debug", got)
	}
	if got := os.Getenv("SOURCE_NAME"); got != "raspotify pi" {
		t.Fatalf("SOURCE_NAME = %q, want raspotify pi", got)
	}
	if got := os.Getenv("RECEIVER_URL"); got != "http://receiver.local" {
		t.Fatalf("RECEIVER_URL = %q, want http://receiver.local", got)
	}
}

func TestLoadDotEnvRejectsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("NOT_VALID\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := LoadDotEnv(path); err == nil {
		t.Fatal("LoadDotEnv returned nil error, want parse failure")
	}
}

func unsetEnvOnCleanup(t *testing.T, keys ...string) {
	t.Helper()

	snapshots := make(map[string]*string, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if !ok {
			snapshots[key] = nil
			continue
		}
		copyValue := value
		snapshots[key] = &copyValue
	}

	for _, key := range keys {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%q): %v", key, err)
		}
	}

	t.Cleanup(func() {
		for _, key := range keys {
			if snapshots[key] == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *snapshots[key])
		}
	})
}
