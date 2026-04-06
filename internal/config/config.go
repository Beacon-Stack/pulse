package config

// Config holds all application configuration.
// Values are loaded from config.yaml and can be overridden by
// CONFIGURARR_* environment variables (e.g. CONFIGURARR_SERVER_PORT=9696).
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Log      LogConfig      `mapstructure:"log"`
	Auth     AuthConfig     `mapstructure:"auth"`

	// ConfigFile is the path of the config file that was loaded, if any.
	ConfigFile string `mapstructure:"-"`
}

// ServerConfig controls the HTTP server.
type ServerConfig struct {
	Host        string `mapstructure:"host"`
	Port        int    `mapstructure:"port"`
	ExternalURL string `mapstructure:"external_url"` // e.g., "http://configurarr:9696" — for Torznab proxy URLs
}

// DatabaseConfig selects and configures the database driver.
type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	Path   string `mapstructure:"path"`
	DSN    Secret `mapstructure:"dsn"`
}

// LogConfig controls log output format and verbosity.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// AuthConfig holds the Configurarr API key used to authenticate requests.
type AuthConfig struct {
	APIKey Secret `mapstructure:"api_key"`
}

// Secret is a string value that is redacted when printed.
type Secret string

// Value returns the underlying string.
func (s Secret) Value() string { return string(s) }

// String redacts the value.
func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return "********"
}
