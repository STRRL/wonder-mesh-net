package coordinator

import "github.com/strrl/wonder-mesh-net/pkg/oidc"

// Config holds configuration for the coordinator server.
type Config struct {
	// Listen is the address the coordinator HTTP server binds to (e.g., ":9080").
	Listen string `mapstructure:"listen"`
	// PublicURL is the externally accessible URL for OAuth callbacks and join tokens.
	PublicURL string `mapstructure:"public_url"`
	// JWTSecret is the signing key for join tokens. If empty, a random one is generated.
	JWTSecret string `mapstructure:"jwt_secret"`
	// GithubClientID is the OAuth client ID for GitHub authentication.
	GithubClientID string `mapstructure:"github_client_id"`
	// GithubClientSecret is the OAuth client secret for GitHub authentication.
	GithubClientSecret string `mapstructure:"github_client_secret"`
	// GoogleClientID is the OAuth client ID for Google authentication.
	GoogleClientID string `mapstructure:"google_client_id"`
	// GoogleClientSecret is the OAuth client secret for Google authentication.
	GoogleClientSecret string `mapstructure:"google_client_secret"`
	// OIDCIssuer is the issuer URL for generic OIDC provider.
	OIDCIssuer string `mapstructure:"oidc_issuer"`
	// OIDCClientID is the client ID for generic OIDC provider.
	OIDCClientID string `mapstructure:"oidc_client_id"`
	// OIDCClientSecret is the client secret for generic OIDC provider.
	OIDCClientSecret string `mapstructure:"oidc_client_secret"`
}

// OIDCProviders returns the configured OIDC providers based on the config fields.
// RedirectURL is set to {PublicURL}/coordinator/auth/callback for all providers.
func (c *Config) OIDCProviders() []oidc.ProviderConfig {
	var providers []oidc.ProviderConfig

	redirectURL := c.PublicURL + "/coordinator/auth/callback"

	if c.GithubClientID != "" {
		providers = append(providers, oidc.ProviderConfig{
			Type:         "github",
			Name:         "github",
			ClientID:     c.GithubClientID,
			ClientSecret: c.GithubClientSecret,
			RedirectURL:  redirectURL,
		})
	}

	if c.GoogleClientID != "" {
		providers = append(providers, oidc.ProviderConfig{
			Type:         "google",
			Name:         "google",
			ClientID:     c.GoogleClientID,
			ClientSecret: c.GoogleClientSecret,
			RedirectURL:  redirectURL,
		})
	}

	if c.OIDCIssuer != "" {
		providers = append(providers, oidc.ProviderConfig{
			Type:         "oidc",
			Name:         "oidc",
			Issuer:       c.OIDCIssuer,
			ClientID:     c.OIDCClientID,
			ClientSecret: c.OIDCClientSecret,
			RedirectURL:  redirectURL,
		})
	}

	return providers
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
