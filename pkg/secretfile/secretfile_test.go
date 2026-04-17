package secretfile

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

func TestOverrideDSNPassword_EmptyPwFile_ReturnsUnchanged(t *testing.T) {
	dsn := "postgres://user:old@host:5432/db?sslmode=disable"
	got, err := OverrideDSNPassword(dsn, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != dsn {
		t.Fatalf("OverrideDSNPassword(dsn, \"\") = %q; want dsn unchanged", got)
	}
}

func TestOverrideDSNPassword_ReplacesPassword(t *testing.T) {
	pwPath := writeSecret(t, "newpass\n")
	dsn := "postgres://user:old@host:5432/db?sslmode=disable"
	got, err := OverrideDSNPassword(dsn, pwPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://user:newpass@host:5432/db?sslmode=disable"
	if got != want {
		t.Fatalf("OverrideDSNPassword = %q; want %q", got, want)
	}
}

func TestOverrideDSNPassword_AddsPasswordWhenDSNHasNone(t *testing.T) {
	pwPath := writeSecret(t, "newpass")
	dsn := "postgres://user@host:5432/db"
	got, err := OverrideDSNPassword(dsn, pwPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://user:newpass@host:5432/db"
	if got != want {
		t.Fatalf("OverrideDSNPassword = %q; want %q", got, want)
	}
}

func TestOverrideDSNPassword_URLEncodesSpecialChars(t *testing.T) {
	pwPath := writeSecret(t, "p@ss w/ord!")
	dsn := "postgres://user:old@host:5432/db"
	got, err := OverrideDSNPassword(dsn, pwPath)
	if err != nil {
		t.Fatal(err)
	}
	// The raw special characters must not appear unencoded.
	if strings.Contains(got, "p@ss w/ord!") {
		t.Fatalf("password should be URL-encoded; got raw in %q", got)
	}
	// And the result must be round-trippable — parsing it back must yield
	// the original password verbatim.
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("result DSN does not re-parse: %v", err)
	}
	if pw, ok := u.User.Password(); !ok || pw != "p@ss w/ord!" {
		t.Fatalf("round-trip password = %q (ok=%v); want %q", pw, ok, "p@ss w/ord!")
	}
}

func TestOverrideDSNPassword_NoUserInDSN_Errors(t *testing.T) {
	pwPath := writeSecret(t, "newpass")
	_, err := OverrideDSNPassword("postgres://host:5432/db", pwPath)
	if err == nil {
		t.Fatal("expected error when DSN has no user component")
	}
}

func TestOverrideDSNPassword_MissingPwFile_Errors(t *testing.T) {
	_, err := OverrideDSNPassword("postgres://user:old@host:5432/db", "/nonexistent/path")
	if err == nil {
		t.Fatal("expected error when password file is missing")
	}
}

func TestOverrideDSNPassword_PreservesQueryParams(t *testing.T) {
	pwPath := writeSecret(t, "newpass")
	dsn := "postgres://user:old@host:5432/db?sslmode=disable&pool_max_conns=10"
	got, err := OverrideDSNPassword(dsn, pwPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "sslmode=disable") || !strings.Contains(got, "pool_max_conns=10") {
		t.Fatalf("query params not preserved in %q", got)
	}
}
