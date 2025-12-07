package coordinator

import "github.com/strrl/wonder-mesh-net/pkg/oidc"

type Config struct {
	ListenAddr    string
	PublicURL     string
	JWTSecret     string
	OIDCProviders []oidc.ProviderConfig
}

const (
	DefaultHeadscaleBinary    = "headscale"
	DefaultHeadscaleConfigDir = "/etc/headscale"
	DefaultDataDir            = "/data"
	DefaultHeadscaleDataDir   = "/data/headscale"
	DefaultCoordinatorDataDir = "/data/coordinator"
	DefaultDatabasePath       = "/data/coordinator/coordinator.db"
	DefaultHeadscaleURL       = "http://127.0.0.1:8080"
	DefaultHeadscaleGRPCAddr  = "127.0.0.1:50443"
)
