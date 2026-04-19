package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const testFixture = `
server:
  port: 9696
database:
  driver: postgres
  dsn: "postgres://user:plain@host:5432/db"
auth:
  api_key: "test-key"
`

func writeFixture(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_PasswordFileOverridesDSN(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeFixture(t, dir, "config.yaml", testFixture)
	pwFile := writeFixture(t, dir, "pw.txt", "secretpw\n")

	t.Setenv("PULSE_DATABASE_PASSWORD_FILE", pwFile)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := "postgres://user:secretpw@host:5432/db"
	if got := cfg.Database.DSN.Value(); got != want {
		t.Fatalf("DSN = %q; want %q", got, want)
	}
}

func TestLoad_NoPasswordFile_LeavesDSNIntact(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeFixture(t, dir, "config.yaml", testFixture)

	t.Setenv("PULSE_DATABASE_PASSWORD_FILE", "")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := "postgres://user:plain@host:5432/db"
	if got := cfg.Database.DSN.Value(); got != want {
		t.Fatalf("DSN = %q; want %q", got, want)
	}
}

func TestLoad_InvalidPasswordFilePath_Errors(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeFixture(t, dir, "config.yaml", testFixture)

	t.Setenv("PULSE_DATABASE_PASSWORD_FILE", "/nonexistent/secret")

	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error when password file path is invalid")
	}
}

// ── EnsureAPIKey ────────────────────────────────────────────────────────────

// stubStore is a minimal APIKeyStore for exercising EnsureAPIKey without
// standing up a real Postgres.
type stubStore struct {
	stored   string
	setErr   error
	setCalls int
}

func (s *stubStore) GetAPIKey(_ context.Context) (string, error) { return s.stored, nil }

func (s *stubStore) SetAPIKey(_ context.Context, value string) error {
	s.setCalls++
	if s.setErr != nil {
		return s.setErr
	}
	s.stored = value
	return nil
}

func TestEnsureAPIKey_EnvOverrideWins(t *testing.T) {
	cfg := &Config{}
	cfg.Auth.APIKey = "env-provided"
	store := &stubStore{stored: "stale-db-value"}

	generated, err := EnsureAPIKey(context.Background(), store, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if generated {
		t.Error("should not generate when key already set")
	}
	if cfg.Auth.APIKey.Value() != "env-provided" {
		t.Errorf("cfg.Auth.APIKey = %q; want env-provided", cfg.Auth.APIKey.Value())
	}
	if store.setCalls != 0 {
		t.Error("should not write to DB when env override is in effect")
	}
}

func TestEnsureAPIKey_LoadsFromDB(t *testing.T) {
	cfg := &Config{}
	store := &stubStore{stored: "persisted-key-from-db"}

	generated, err := EnsureAPIKey(context.Background(), store, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if generated {
		t.Error("should not generate when DB has a key")
	}
	if cfg.Auth.APIKey.Value() != "persisted-key-from-db" {
		t.Errorf("cfg.Auth.APIKey = %q; want persisted-key-from-db", cfg.Auth.APIKey.Value())
	}
	if store.setCalls != 0 {
		t.Error("should not re-write the existing key")
	}
}

func TestEnsureAPIKey_GeneratesAndStores(t *testing.T) {
	cfg := &Config{}
	store := &stubStore{}

	generated, err := EnsureAPIKey(context.Background(), store, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Error("should generate when DB is empty")
	}
	if len(cfg.Auth.APIKey.Value()) != 64 {
		t.Errorf("generated key length = %d; want 64 (hex-encoded 32 bytes)", len(cfg.Auth.APIKey.Value()))
	}
	if store.setCalls != 1 {
		t.Errorf("SetAPIKey calls = %d; want 1", store.setCalls)
	}
	if store.stored != cfg.Auth.APIKey.Value() {
		t.Errorf("DB value (%q) doesn't match cfg.Auth.APIKey (%q)", store.stored, cfg.Auth.APIKey.Value())
	}
}

func TestEnsureAPIKey_PropagatesStoreWriteError(t *testing.T) {
	cfg := &Config{}
	store := &stubStore{setErr: errors.New("disk full")}

	_, err := EnsureAPIKey(context.Background(), store, cfg)
	if err == nil {
		t.Fatal("expected error when store.Set fails")
	}
}

func TestLoad_APIKeyFileOverridesInlineKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeFixture(t, dir, "config.yaml", testFixture) // api_key: "test-key"
	keyFile := writeFixture(t, dir, "pulse.txt", "file-wins\n")

	t.Setenv("PULSE_AUTH_API_KEY_FILE", keyFile)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Auth.APIKey.Value(); got != "file-wins" {
		t.Fatalf("APIKey = %q; want %q (file should override the config-file value)", got, "file-wins")
	}
}

func TestLoad_InvalidAPIKeyFilePath_Errors(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeFixture(t, dir, "config.yaml", testFixture)

	t.Setenv("PULSE_AUTH_API_KEY_FILE", "/nonexistent/pulse-api-key")

	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error when api_key_file path is invalid")
	}
}
