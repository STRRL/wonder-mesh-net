package coordinator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/store"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const minJWTSecretLength = 32

// Server is the coordinator server that manages multi-realm Headscale access.
type Server struct {
	config                  *Config
	db                      *database.Manager
	headscaleConn           *grpc.ClientConn
	headscaleClient         v1.HeadscaleServiceClient
	headscaleProcessManager *headscale.ProcessManager
	realmManager            *headscale.RealmManager
	aclManager              *headscale.ACLManager
	oidcRegistry            *oidc.Registry
	tokenGenerator          *jointoken.Generator
	sessionStore            *store.DBSessionStore
	userStore               *store.DBUserStore
	apiKeyStore             *store.DBAPIKeyStore
	deviceFlowStore         *store.DeviceRequestStore
}

// BootstrapNewServer creates a new coordinator server.
func BootstrapNewServer(config *Config) (*Server, error) {
	if len(config.JWTSecret) < minJWTSecretLength {
		return nil, fmt.Errorf("JWT secret must be at least %d bytes", minJWTSecretLength)
	}

	if err := os.MkdirAll(DefaultCoordinatorDataDir, 0755); err != nil {
		return nil, fmt.Errorf("create coordinator data dir: %w", err)
	}
	if err := os.MkdirAll(DefaultHeadscaleDataDir, 0755); err != nil {
		return nil, fmt.Errorf("create headscale data dir: %w", err)
	}

	db, err := database.NewManager(database.Config{
		Driver: database.DriverSQLite,
		DSN:    DefaultDatabaseDSN,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	slog.Info("database initialized", "driver", database.DriverSQLite, "dsn", DefaultDatabaseDSN)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	slog.Info("starting embedded Headscale")

	configPath := filepath.Join(DefaultHeadscaleConfigDir, "config.yaml")
	headscaleProcessManager := headscale.NewProcessManager(headscale.ProcessConfig{
		BinaryPath: DefaultHeadscaleBinary,
		ConfigPath: configPath,
		DataDir:    DefaultHeadscaleDataDir,
	})

	if err := headscaleProcessManager.Start(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("start headscale: %w", err)
	}

	if err := headscaleProcessManager.WaitReady(ctx, 30*time.Second); err != nil {
		_ = headscaleProcessManager.Stop()
		_ = db.Close()
		return nil, fmt.Errorf("headscale not ready: %w", err)
	}

	slog.Info("Headscale started successfully")

	apiKey, err := headscaleProcessManager.CreateAPIKey(ctx)
	if err != nil {
		_ = headscaleProcessManager.Stop()
		_ = db.Close()
		return nil, fmt.Errorf("create headscale API key: %w", err)
	}
	slog.Info("Headscale API key created")

	headscaleConn, err := grpc.NewClient(
		DefaultHeadscaleGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&headscale.APIKeyCredentials{APIKey: apiKey}),
	)
	if err != nil {
		_ = headscaleProcessManager.Stop()
		_ = db.Close()
		return nil, fmt.Errorf("connect to headscale gRPC: %w", err)
	}
	headscaleClient := v1.NewHeadscaleServiceClient(headscaleConn)

	stateStore := store.NewDBAuthStateStore(db.Queries(), 10*time.Minute)
	oidcRegistry := oidc.NewRegistryWithStore(stateStore)

	for _, providerConfig := range config.OIDCProviders() {
		if err := oidcRegistry.RegisterProvider(ctx, providerConfig); err != nil {
			slog.Warn("register OIDC provider", "provider", providerConfig.Name, "error", err)
		} else {
			slog.Info("registered OIDC provider", "provider", providerConfig.Name)
		}
	}

	// Both coordinatorURL and headscaleURL use PublicURL because the coordinator
	// reverse-proxies Tailscale control plane traffic to embedded Headscale.
	tokenGenerator := jointoken.NewGenerator(
		config.JWTSecret,
		config.PublicURL,
		config.PublicURL,
	)

	return &Server{
		config:                  config,
		db:                      db,
		headscaleConn:           headscaleConn,
		headscaleClient:         headscaleClient,
		headscaleProcessManager: headscaleProcessManager,
		realmManager:            headscale.NewRealmManager(headscaleClient),
		aclManager:              headscale.NewACLManager(headscaleClient),
		oidcRegistry:            oidcRegistry,
		tokenGenerator:          tokenGenerator,
		sessionStore:            store.NewDBSessionStore(db.Queries()),
		userStore:               store.NewDBUserStore(db.Queries()),
		apiKeyStore:             store.NewDBAPIKeyStore(db.Queries()),
		deviceFlowStore:         store.NewDeviceRequestStore(db.Queries()),
	}, nil
}

// Close closes all server resources
func (s *Server) Close() error {
	if s.headscaleConn != nil {
		_ = s.headscaleConn.Close()
	}
	if s.headscaleProcessManager != nil {
		if err := s.headscaleProcessManager.Stop(); err != nil {
			slog.Warn("stop headscale", "error", err)
		}
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
