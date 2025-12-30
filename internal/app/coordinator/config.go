package coordinator

// Config holds configuration for the coordinator server.
type Config struct {
	// Listen is the address the coordinator HTTP server binds to (e.g., ":9080").
	Listen string `mapstructure:"listen"`
	// PublicURL is the externally accessible URL for OAuth callbacks and join tokens.
	PublicURL string `mapstructure:"public_url"`
	// JWTSecret is the signing key for join tokens. If empty, a random one is generated.
	JWTSecret string `mapstructure:"jwt_secret"`

	// KeycloakURL is the base URL of the Keycloak server (e.g., "https://auth.example.com").
	KeycloakURL string `mapstructure:"keycloak_url"`
	// KeycloakRealm is the Keycloak realm for user authentication (e.g., "wonder-mesh").
	KeycloakRealm string `mapstructure:"keycloak_realm"`
	// KeycloakClientID is the OIDC client ID for the coordinator (used for audience validation).
	KeycloakClientID string `mapstructure:"keycloak_client_id"`
}

const (
	DefaultHeadscaleBinary    = "headscale"
	DefaultHeadscaleConfigDir = "/etc/headscale"
	DefaultDataDir            = "/data"
	DefaultHeadscaleDataDir   = "/data/headscale"
	DefaultCoordinatorDataDir = "/data/coordinator"
	DefaultDatabaseDSN        = "file:/data/coordinator/coordinator.db?_journal_mode=WAL&_busy_timeout=5000"
	DefaultHeadscaleURL       = "http://127.0.0.1:8080"
	DefaultHeadscaleGRPCAddr  = "127.0.0.1:50443"
)
