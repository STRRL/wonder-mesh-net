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
	"github.com/strrl/wonder-mesh-net/pkg/jwtauth"
	"github.com/strrl/wonder-mesh-net/pkg/keycloak"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const minJWTSecretLength = 32

// Server is the coordinator server that manages multi-tenant wonder net access.
type Server struct {
	config                  *Config
	db                      *database.Manager
	headscaleConn           *grpc.ClientConn
	headscaleClient         v1.HeadscaleServiceClient
	headscaleProcessManager *headscale.ProcessManager

	// Auth components
	jwtValidator    *jwtauth.Validator
	keycloakClient  *keycloak.AdminClient

	// Repositories
	userRepository       *repository.UserRepository
	wonderNetRepository  *repository.WonderNetRepository
	deviceFlowRepository *repository.DeviceRequestRepository

	// Services
	wonderNetService    *service.WonderNetService
	workerService       *service.WorkerService
	deviceFlowService   *service.DeviceFlowService
	nodesService        *service.NodesService
	keycloakAuthService *service.KeycloakAuthService
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

	// Both coordinatorURL and headscaleURL use PublicURL because the coordinator
	// reverse-proxies Tailscale control plane traffic to embedded Headscale.
	tokenGenerator := jointoken.NewGenerator(
		config.JWTSecret,
		config.PublicURL,
		config.PublicURL,
	)

	// Create repositories
	userRepository := repository.NewUserRepository(db.Queries())
	wonderNetRepository := repository.NewWonderNetRepository(db.Queries())
	serviceAccountRepository := repository.NewServiceAccountRepository(db.Queries())
	deviceFlowRepository := repository.NewDeviceRequestRepository(db.Queries())

	// Create Headscale managers
	wonderNetManager := headscale.NewWonderNetManager(headscaleClient)
	aclManager := headscale.NewACLManager(headscaleClient)

	// Create services
	wonderNetService := service.NewWonderNetService(wonderNetRepository, wonderNetManager, aclManager, config.PublicURL)
	workerService := service.NewWorkerService(tokenGenerator, config.JWTSecret, wonderNetRepository, wonderNetService)
	deviceFlowService := service.NewDeviceFlowService(deviceFlowRepository, wonderNetService, config.PublicURL)
	nodesService := service.NewNodesService(wonderNetManager)

	// Create Keycloak admin client
	keycloakClient := keycloak.NewAdminClient(keycloak.AdminClientConfig{
		URL:          config.KeycloakURL,
		Realm:        config.KeycloakRealm,
		ClientID:     config.KeycloakAdminClient,
		ClientSecret: config.KeycloakAdminSecret,
	})

	// Authenticate Keycloak admin client
	if err := keycloakClient.Authenticate(ctx); err != nil {
		_ = headscaleProcessManager.Stop()
		_ = db.Close()
		return nil, fmt.Errorf("authenticate keycloak admin client: %w", err)
	}
	slog.Info("authenticated with Keycloak admin API")

	// Create Keycloak auth service
	keycloakAuthService := service.NewKeycloakAuthService(keycloakClient, userRepository, wonderNetRepository, serviceAccountRepository, wonderNetService)

	// Create JWT validator for Keycloak tokens
	jwksURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/certs", config.KeycloakURL, config.KeycloakRealm)
	issuer := fmt.Sprintf("%s/realms/%s", config.KeycloakURL, config.KeycloakRealm)
	jwtValidator := jwtauth.NewValidator(jwtauth.ValidatorConfig{
		JWKSURL:         jwksURL,
		Issuer:          issuer,
		Audience:        config.KeycloakClientID,
		RefreshInterval: 5 * time.Minute,
	})

	// Start JWT validator background refresh
	if err := jwtValidator.Start(ctx); err != nil {
		_ = headscaleProcessManager.Stop()
		_ = db.Close()
		return nil, fmt.Errorf("start JWT validator: %w", err)
	}
	slog.Info("JWT validator started", "jwks_url", jwksURL)

	return &Server{
		config:                  config,
		db:                      db,
		headscaleConn:           headscaleConn,
		headscaleClient:         headscaleClient,
		headscaleProcessManager: headscaleProcessManager,
		jwtValidator:            jwtValidator,
		keycloakClient:          keycloakClient,
		userRepository:          userRepository,
		wonderNetRepository:     wonderNetRepository,
		deviceFlowRepository:    deviceFlowRepository,
		wonderNetService:        wonderNetService,
		workerService:           workerService,
		deviceFlowService:       deviceFlowService,
		nodesService:            nodesService,
		keycloakAuthService:     keycloakAuthService,
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
