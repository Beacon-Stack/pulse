package file

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFile_WritesJSONLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	p, err := New(Config{Path: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close(context.Background())

	logger := slog.New(p.Handler())
	logger.Info("hello", "n", 42)

	// File handle is closed in Close, but we want to read while open;
	// just sync via the os.File doesn't expose to API, so close + reopen.
	_ = p.Close(context.Background())

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	line := strings.TrimSpace(string(data))
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("not JSON: %v line=%q", err, line)
	}
	if rec["msg"] != "hello" || rec["n"] != float64(42) {
		t.Errorf("unexpected payload: %+v", rec)
	}
}

func TestFile_RotatesOnSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rot.log")

	// Tiny MaxBytes so a few writes trigger rotation.
	p, err := New(Config{Path: path, MaxBytes: 100, MaxFiles: 2})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close(context.Background())

	logger := slog.New(p.Handler())
	for i := 0; i < 50; i++ {
		logger.Info("padding line to force rotation", "i", i)
	}
	_ = p.Close(context.Background())

	// At least one rotated file should exist (.1).
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("expected rotation slot %s.1 to exist after writes; stat err: %v", path, err)
	}
}

func TestFile_RequiresPath(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Error("expected error for empty Path")
	}
}
