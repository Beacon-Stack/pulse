package config

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	"github.com/beacon-stack/pulse/pkg/secretfile"
)

// dataDir returns the default data directory: ~/.config/pulse/
func dataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "data" // fallback to relative
	}
	return filepath.Join(home, ".config", "pulse")
}

// Load reads configuration from file, env, and flags, then returns a Config.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	dir := dataDir()

	// Defaults — use absolute paths so data survives CWD changes.
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 9696)
	v.SetDefault("server.external_url", "")
	v.SetDefault("database.driver", "postgres")
	v.SetDefault("database.dsn", "")
	v.SetDefault("database.password_file", "")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("flaresolverr.url", "")

	// Env vars: PULSE_SERVER_PORT, etc.
	v.SetEnvPrefix("PULSE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Config file search order:
	//   1. Explicit path via --config flag
	//   2. ~/.config/pulse/config.yaml
	//   3. /config/config.yaml (Docker volume mount)
	//   4. ./config.yaml (CWD fallback)
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(dir)
		v.AddConfigPath("/config")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && configPath != "" {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if cfg.Database.PasswordFile != "" {
		merged, err := secretfile.OverrideDSNPassword(cfg.Database.DSN.Value(), cfg.Database.PasswordFile)
		if err != nil {
			return nil, fmt.Errorf("applying database password file: %w", err)
		}
		cfg.Database.DSN = Secret(merged)
	}

	if v.ConfigFileUsed() != "" {
		cfg.ConfigFile = v.ConfigFileUsed()
	}

	// The API key is no longer auto-generated here. Load() may leave
	// cfg.Auth.APIKey empty; the caller (main.go) resolves it against the
	// DB via EnsureAPIKey after Postgres is up. The env var and config-file
	// values (if set) still flow through and take priority.
	return &cfg, nil
}

// EnsureAPIKey makes sure cfg.Auth.APIKey is set, persisting it in the
// shared config_entries table so it survives container restarts. Called
// from main after the DB is open and the config store is available.
//
// Priority (first match wins):
//  1. cfg.Auth.APIKey already set — from PULSE_AUTH_API_KEY env var or a
//     loaded config.yaml. Treated as an ops override; not written to DB.
//  2. config_entries row at (namespace="auth", key="api_key"). Loaded and
//     assigned.
//  3. Generate a new key, INSERT it into config_entries, use it.
//
// Returns (generated=true) if a fresh key was created.
func EnsureAPIKey(ctx context.Context, store APIKeyStore, cfg *Config) (generated bool, err error) {
	if cfg.Auth.APIKey != "" {
		return false, nil
	}

	if existing, err := store.GetAPIKey(ctx); err == nil && existing != "" {
		cfg.Auth.APIKey = Secret(existing)
		return false, nil
	}

	key, err := generateAPIKey()
	if err != nil {
		return false, fmt.Errorf("generating API key: %w", err)
	}
	if err := store.SetAPIKey(ctx, key); err != nil {
		return false, fmt.Errorf("persisting API key: %w", err)
	}
	cfg.Auth.APIKey = Secret(key)
	return true, nil
}

// APIKeyStore is the narrow interface EnsureAPIKey needs from the shared
// config store. Wired up in main.go by the cfgstore.Store implementation.
type APIKeyStore interface {
	GetAPIKey(ctx context.Context) (string, error)
	SetAPIKey(ctx context.Context, value string) error
}

// WriteDefault writes a default config file to the given path.
func WriteDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	key, _ := generateAPIKey()
	content := fmt.Sprintf(`# Pulse configuration
server:
  host: "0.0.0.0"
  port: 9696

database:
  driver: postgres
  dsn: "postgres://pulse:pulse@localhost:5432/pulse_db?sslmode=disable"

log:
  level: info
  format: json

auth:
  api_key: "%s"
`, key)

	return os.WriteFile(path, []byte(content), 0o600)
}

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
