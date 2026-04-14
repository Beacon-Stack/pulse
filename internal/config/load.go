package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
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

	if v.ConfigFileUsed() != "" {
		cfg.ConfigFile = v.ConfigFileUsed()
	}

	// Auto-generate API key if not set, and persist it to the config file
	// so it survives restarts.
	if cfg.Auth.APIKey == "" {
		key, err := generateAPIKey()
		if err != nil {
			return nil, fmt.Errorf("generating API key: %w", err)
		}
		cfg.Auth.APIKey = Secret(key)

		// Ensure config directory exists and write/update the config file.
		cfgFile := cfg.ConfigFile
		if cfgFile == "" {
			cfgFile = filepath.Join(dir, "config.yaml")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("creating config directory: %w", err)
			}
		}
		v.Set("auth.api_key", key)
		v.SetConfigFile(cfgFile)
		if err := v.WriteConfig(); err != nil {
			// Try SafeWriteConfig for first-time creation.
			_ = v.SafeWriteConfig()
		}
		cfg.ConfigFile = cfgFile
	}

	return &cfg, nil
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
