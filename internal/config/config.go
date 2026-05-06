package config

// Config holds all application configuration.
// Values are loaded from config.yaml and can be overridden by
// PULSE_* environment variables (e.g. PULSE_SERVER_PORT=9696).
type Config struct {
	Server       ServerConfig       `mapstructure:"server"`
	Database     DatabaseConfig     `mapstructure:"database"`
	Log          LogConfig          `mapstructure:"log"`
	Auth         AuthConfig         `mapstructure:"auth"`
	FlareSolverr FlareSolverrConfig `mapstructure:"flaresolverr"`
	Dashboard    DashboardConfig    `mapstructure:"dashboard"`

	// ConfigFile is the path of the config file that was loaded, if any.
	ConfigFile string `mapstructure:"-"`
}

// DashboardConfig opts the dashboard into Docker-stats and Gluetun
// integrations. Both default off — the dashboard works without them, just
// without container resource info or VPN status panels.
type DashboardConfig struct {
	// DockerSocket is the path to a mounted Docker socket. Empty disables
	// container stats entirely. The socket is read-only on disk but still
	// effectively grants root on most Docker builds — opt in only.
	DockerSocket string `mapstructure:"docker_socket"`

	// GluetunURL is the base URL of a Gluetun container's control server
	// (e.g. "http://vpn:8000"). Empty disables the VPN panel.
	GluetunURL string `mapstructure:"gluetun_url"`

	// GluetunContainer is the docker container name for Gluetun. Defaults
	// to "vpn" when unset, matching deploy/docker-compose.vpn.yml.
	GluetunContainer string `mapstructure:"gluetun_container"`

	// ContainerNames overrides the default service-name → container-name
	// mapping for non-standard deployments (e.g. Haul → haul-prod).
	ContainerNames map[string]string `mapstructure:"container_names"`
}

// FlareSolverrConfig holds optional FlareSolverr proxy settings.
// When URL is set, Pulse will use FlareSolverr to bypass Cloudflare
// challenges on protected indexer sites.
type FlareSolverrConfig struct {
	// URL is the FlareSolverr endpoint (e.g., "http://localhost:8191").
	// Empty means disabled.
	URL string `mapstructure:"url"`
}

// ServerConfig controls the HTTP server.
type ServerConfig struct {
	Host        string `mapstructure:"host"`
	Port        int    `mapstructure:"port"`
	ExternalURL string `mapstructure:"external_url"` // e.g., "http://pulse:9696" — for Torznab proxy URLs
}

// DatabaseConfig selects and configures the database driver.
type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	DSN    Secret `mapstructure:"dsn"`

	// PasswordFile is a path to a file containing the database password,
	// typically a Docker secret mounted at /run/secrets/*. When non-empty,
	// its contents replace the password component of DSN at load time.
	PasswordFile string `mapstructure:"password_file"`
}

// LogConfig controls log output format and verbosity.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// AuthConfig holds the Pulse API key used to authenticate requests.
type AuthConfig struct {
	APIKey Secret `mapstructure:"api_key"`

	// APIKeyFile is a path to a file containing the API key, typically a
	// Docker secret mounted at /run/secrets/*. When non-empty, its contents
	// replace APIKey at load time and win over the DB config_entries row.
	APIKeyFile string `mapstructure:"api_key_file"`
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
