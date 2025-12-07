package coordinator

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"github.com/strrl/wonder-mesh-net/pkg/database"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Server is the coordinator server that manages multi-tenant Headscale access.
type Server struct {
	Config         *Config
	DB             *database.Manager
	HSConn         *grpc.ClientConn
	HSClient       v1.HeadscaleServiceClient
	HSProcess      *headscale.ProcessManager
	TenantManager  *headscale.TenantManager
	ACLManager     *headscale.ACLManager
	OIDCRegistry   *oidc.Registry
	TokenGenerator *jointoken.Generator
	SessionStore   *oidc.DBSessionStore
	UserStore      *oidc.DBUserStore
}

// NewServer creates a new coordinator server.
func NewServer(config *Config) (*Server, error) {
	if err := os.MkdirAll(DefaultCoordinatorDataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create coordinator data dir: %w", err)
	}
	if err := os.MkdirAll(DefaultHeadscaleDataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create headscale data dir: %w", err)
	}

	db, err := database.NewManager(DefaultDatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	log.Printf("Database initialized at %s", DefaultDatabasePath)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log.Printf("Starting embedded Headscale...")

	configPath := filepath.Join(DefaultHeadscaleConfigDir, "config.yaml")
	hsProcess := headscale.NewProcessManager(headscale.ProcessConfig{
		BinaryPath: DefaultHeadscaleBinary,
		ConfigPath: configPath,
		DataDir:    DefaultHeadscaleDataDir,
	})

	if err := hsProcess.Start(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to start headscale: %w", err)
	}

	if err := hsProcess.WaitReady(ctx, 30*time.Second); err != nil {
		_ = hsProcess.Stop()
		_ = db.Close()
		return nil, fmt.Errorf("headscale not ready: %w", err)
	}

	log.Printf("Headscale started successfully")

	apiKey, err := hsProcess.CreateAPIKey(ctx)
	if err != nil {
		_ = hsProcess.Stop()
		_ = db.Close()
		return nil, fmt.Errorf("failed to create headscale API key: %w", err)
	}
	log.Printf("Headscale API key created")

	hsConn, err := grpc.NewClient(
		DefaultHeadscaleGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&headscale.APIKeyCredentials{APIKey: apiKey}),
	)
	if err != nil {
		_ = hsProcess.Stop()
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect to headscale gRPC: %w", err)
	}
	hsClient := v1.NewHeadscaleServiceClient(hsConn)

	stateStore := oidc.NewDBAuthStateStore(db.Queries(), 10*time.Minute)
	oidcRegistry := oidc.NewRegistryWithStore(stateStore)

	for _, providerConfig := range config.OIDCProviders {
		if err := oidcRegistry.RegisterProvider(ctx, providerConfig); err != nil {
			log.Printf("Warning: failed to register OIDC provider %s: %v", providerConfig.Name, err)
		} else {
			log.Printf("Registered OIDC provider: %s", providerConfig.Name)
		}
	}

	tokenGenerator := jointoken.NewGenerator(
		config.JWTSecret,
		config.PublicURL,
		config.PublicURL,
	)

	return &Server{
		Config:         config,
		DB:             db,
		HSConn:         hsConn,
		HSClient:       hsClient,
		HSProcess:      hsProcess,
		TenantManager:  headscale.NewTenantManager(hsClient),
		ACLManager:     headscale.NewACLManager(hsClient),
		OIDCRegistry:   oidcRegistry,
		TokenGenerator: tokenGenerator,
		SessionStore:   oidc.NewDBSessionStore(db.Queries()),
		UserStore:      oidc.NewDBUserStore(db.Queries()),
	}, nil
}

// Close closes all server resources
func (s *Server) Close() error {
	if s.HSConn != nil {
		_ = s.HSConn.Close()
	}
	if s.HSProcess != nil {
		if err := s.HSProcess.Stop(); err != nil {
			log.Printf("Warning: failed to stop headscale: %v", err)
		}
	}
	if s.DB != nil {
		return s.DB.Close()
	}
	return nil
}
