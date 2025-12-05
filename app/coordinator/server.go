package coordinator

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

// Server is the coordinator server that manages multi-tenant Headscale access.
type Server struct {
	Config         *Config
	HSClient       *headscale.Client
	TenantManager  *headscale.TenantManager
	ACLManager     *headscale.ACLManager
	OIDCRegistry   *oidc.Registry
	TokenGenerator *jointoken.Generator
}

// NewServer creates a new coordinator server.
func NewServer(config *Config) (*Server, error) {
	hsClient := headscale.NewClient(config.HeadscaleURL, config.HeadscaleAPIKey)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := hsClient.Health(ctx); err != nil {
		return nil, fmt.Errorf("headscale health check failed: %w", err)
	}

	oidcRegistry := oidc.NewRegistry()
	for _, providerConfig := range config.OIDCProviders {
		if err := oidcRegistry.RegisterProvider(ctx, providerConfig); err != nil {
			log.Printf("Warning: failed to register OIDC provider %s: %v", providerConfig.Name, err)
		} else {
			log.Printf("Registered OIDC provider: %s", providerConfig.Name)
		}
	}

	headscaleURLForToken := config.HeadscaleURL
	if config.HeadscalePublicURL != "" {
		headscaleURLForToken = config.HeadscalePublicURL
	}
	tokenGenerator := jointoken.NewGenerator(
		config.JWTSecret,
		config.PublicURL,
		headscaleURLForToken,
	)

	return &Server{
		Config:         config,
		HSClient:       hsClient,
		TenantManager:  headscale.NewTenantManager(hsClient),
		ACLManager:     headscale.NewACLManager(hsClient),
		OIDCRegistry:   oidcRegistry,
		TokenGenerator: tokenGenerator,
	}, nil
}
