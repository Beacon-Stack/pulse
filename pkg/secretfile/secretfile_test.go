package secretfile

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSecret(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRead_EmptyPath(t *testing.T) {
	got, err := Read("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("Read(\"\") = %q; want empty string", got)
	}
}

func TestRead_HappyPath(t *testing.T) {
	path := writeSecret(t, "hunter2")
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hunter2" {
		t.Fatalf("Read = %q; want %q", got, "hunter2")
	}
}

func TestRead_TrimsTrailingNewline(t *testing.T) {
	path := writeSecret(t, "hunter2\n")
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hunter2" {
		t.Fatalf("Read = %q; want %q (trailing newline not trimmed)", got, "hunter2")
	}
}

func TestRead_TrimsCRLF(t *testing.T) {
	path := writeSecret(t, "hunter2\r\n")
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hunter2" {
		t.Fatalf("Read = %q; want %q (CRLF not trimmed)", got, "hunter2")
	}
}

func TestRead_PreservesLeadingAndInnerWhitespace(t *testing.T) {
	path := writeSecret(t, " pass word \n")
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != " pass word" {
		t.Fatalf("Read = %q; want leading/inner space preserved", got)
	}
}

func TestRead_MissingFile(t *testing.T) {
	_, err := Read("/nonexistent/path/to/secret")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
