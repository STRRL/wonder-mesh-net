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
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
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

	// Repositories
	apiKeyRepository     *repository.APIKeyRepository
	deviceFlowRepository *repository.DeviceRequestRepository

	// Services
	authService       *service.AuthService
	realmService      *service.RealmService
	oidcService       *service.OIDCService
	workerService     *service.WorkerService
	deviceFlowService *service.DeviceFlowService
	nodesService      *service.NodesService
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

	authStateRepository := repository.NewAuthStateRepository(db.Queries(), 10*time.Minute)
	oidcRegistry := oidc.NewRegistryWithStore(authStateRepository)

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

	// Create repositories
	userRepository := repository.NewUserRepository(db.Queries())
	sessionRepository := repository.NewSessionRepository(db.Queries())
	realmRepository := repository.NewRealmRepository(db.Queries())
	identityRepository := repository.NewOIDCIdentityRepository(db.Queries())
	apiKeyRepository := repository.NewAPIKeyRepository(db.Queries())
	deviceFlowRepository := repository.NewDeviceRequestRepository(db.Queries())

	// Create services
	realmManager := headscale.NewRealmManager(headscaleClient)
	aclManager := headscale.NewACLManager(headscaleClient)

	authService := service.NewAuthService(sessionRepository, realmRepository, apiKeyRepository)
	realmService := service.NewRealmService(realmRepository, realmManager, aclManager, config.PublicURL)
	oidcService := service.NewOIDCService(oidcRegistry, userRepository, identityRepository, realmRepository, realmService)
	workerService := service.NewWorkerService(tokenGenerator, config.JWTSecret, realmRepository, realmService)
	deviceFlowService := service.NewDeviceFlowService(deviceFlowRepository, realmService, config.PublicURL)
	nodesService := service.NewNodesService(realmManager)

	return &Server{
		config:                  config,
		db:                      db,
		headscaleConn:           headscaleConn,
		headscaleClient:         headscaleClient,
		headscaleProcessManager: headscaleProcessManager,
		apiKeyRepository:        apiKeyRepository,
		deviceFlowRepository:    deviceFlowRepository,
		authService:             authService,
		realmService:            realmService,
		oidcService:             oidcService,
		workerService:           workerService,
		deviceFlowService:       deviceFlowService,
		nodesService:            nodesService,
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
