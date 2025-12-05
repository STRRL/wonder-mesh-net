package coordinator

import "github.com/strrl/wonder-mesh-net/pkg/oidc"

type Config struct {
	ListenAddr         string
	HeadscaleURL       string
	HeadscalePublicURL string
	HeadscaleAPIKey    string
	PublicURL          string
	JWTSecret          string
	OIDCProviders      []oidc.ProviderConfig
}
