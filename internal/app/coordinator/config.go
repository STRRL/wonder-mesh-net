package coordinator

// Config holds configuration for the coordinator server.
type Config struct {
	// Listen is the address the coordinator HTTP server binds to (e.g., ":9080").
	Listen string `mapstructure:"listen"`
	// PublicURL is the externally accessible URL for OAuth callbacks and join tokens.
	PublicURL string `mapstructure:"public_url"`
	// JWTSecret is the signing key for join tokens. If empty, a random one is generated.
	JWTSecret string `mapstructure:"jwt_secret"`

	// DatabaseDriver selects the storage backend (sqlite or postgres).
	DatabaseDriver string `mapstructure:"database_driver"`
	// DatabaseDSN is the database connection string.
	DatabaseDSN string `mapstructure:"database_dsn"`

	// HeadscaleURL is the HTTP URL of the Headscale server (e.g., "http://headscale:8080").
	HeadscaleURL string `mapstructure:"headscale_url"`
	// HeadscaleUnixSocket is the path to Headscale Unix socket (e.g., "/var/run/headscale/headscale.sock").
	HeadscaleUnixSocket string `mapstructure:"headscale_unix_socket"`

	// KeycloakURL is the base URL of the Keycloak server (e.g., "https://auth.example.com").
	KeycloakURL string `mapstructure:"keycloak_url"`
	// KeycloakRealm is the Keycloak realm for user authentication (e.g., "wonder-mesh").
	KeycloakRealm string `mapstructure:"keycloak_realm"`
	// KeycloakClientID is the OIDC client ID for the coordinator (used for audience validation).
	KeycloakClientID string `mapstructure:"keycloak_client_id"`
	// KeycloakClientSecret is the OIDC client secret for the coordinator (used for token exchange).
	KeycloakClientSecret string `mapstructure:"keycloak_client_secret"`

	// EnableAdminAPI enables the admin API endpoints (disabled by default).
	EnableAdminAPI bool `mapstructure:"enable_admin_api"`
	// AdminAPIAuthToken is the bearer token for admin API authentication.
	// Required if EnableAdminAPI is true. Must be at least 32 characters.
	AdminAPIAuthToken string `mapstructure:"admin_api_auth_token"`
}

const (
	DefaultCoordinatorDataDir  = "/data/coordinator"
	DefaultDatabaseDSN         = "file:/data/coordinator/coordinator.db?_journal_mode=WAL&_busy_timeout=5000"
	DefaultHeadscaleURL        = "http://127.0.0.1:8080"
	DefaultHeadscaleUnixSocket = "/var/run/headscale/headscale.sock"
)
