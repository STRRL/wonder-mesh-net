package coordinator

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/controller"
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
	jwtValidator *jwtauth.Validator

	// Repositories
	wonderNetRepository  *repository.WonderNetRepository
	deviceFlowRepository *repository.DeviceRequestRepository

	// Services
	wonderNetService  *service.WonderNetService
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

	// Both coordinatorURL and headscaleURL use PublicURL because the coordinator
	// reverse-proxies Tailscale control plane traffic to embedded Headscale.
	tokenGenerator := jointoken.NewGenerator(
		config.JWTSecret,
		config.PublicURL,
		config.PublicURL,
	)

	// Create repositories
	wonderNetRepository := repository.NewWonderNetRepository(db.Queries())
	serviceAccountRepository := repository.NewServiceAccountRepository(db.Queries())
	deviceFlowRepository := repository.NewDeviceRequestRepository(db.Queries())

	// Create Headscale managers
	wonderNetManager := headscale.NewWonderNetManager(headscaleClient)
	aclManager := headscale.NewACLManager(headscaleClient)

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

	// Create services
	wonderNetService := service.NewWonderNetService(wonderNetRepository, serviceAccountRepository, wonderNetManager, aclManager, keycloakClient, config.PublicURL)
	workerService := service.NewWorkerService(tokenGenerator, config.JWTSecret, wonderNetRepository, wonderNetService)
	deviceFlowService := service.NewDeviceFlowService(deviceFlowRepository, wonderNetService, config.PublicURL)
	nodesService := service.NewNodesService(wonderNetManager)

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
		wonderNetRepository:     wonderNetRepository,
		deviceFlowRepository:    deviceFlowRepository,
		wonderNetService:        wonderNetService,
		workerService:           workerService,
		deviceFlowService:       deviceFlowService,
		nodesService:            nodesService,
	}, nil
}

// requireAuth wraps a handler with JWT authentication.
// It validates the JWT token and adds the claims to the request context.
// This middleware only handles authentication, not WonderNet resolution.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		claims, err := s.jwtValidator.Validate(token)
		if err != nil {
			slog.Debug("JWT validation failed", "error", err)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), jwtauth.ContextKeyClaims, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// requireWonderNet wraps a handler to resolve the WonderNet from JWT claims.
// For regular users, it auto-creates a WonderNet if none exists.
// For service accounts, it looks up the associated WonderNet.
// Must be used after requireAuth.
func (s *Server) requireWonderNet(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := jwtauth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		wonderNet, isServiceAccount, err := s.wonderNetService.ResolveWonderNetFromClaims(r.Context(), claims)
		if err != nil {
			if isServiceAccount {
				slog.Error("get service account wonder net", "error", err)
				http.Error(w, "service account not associated with wonder net", http.StatusUnauthorized)
				return
			}
			slog.Error("get or create wonder net", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		ctx := context.WithValue(r.Context(), controller.ContextKeyWonderNet, wonderNet)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// Run starts the HTTP server and blocks until a shutdown signal is received.
// It registers all API routes, starts listening on the configured address,
// and handles graceful shutdown on SIGINT or SIGTERM with a 10-second timeout.
func (s *Server) Run() error {
	healthController := controller.NewHealthController(s.headscaleClient)
	workerController := controller.NewWorkerController(s.workerService)
	deviceFlowController := controller.NewDeviceFlowController(s.deviceFlowService, s.config.PublicURL, s.config.KeycloakURL, s.config.KeycloakRealm)
	authKeyController := controller.NewAuthKeyController(s.wonderNetService)
	joinTokenController := controller.NewJoinTokenController(s.workerService)
	nodesController := controller.NewNodesController(s.nodesService)
	serviceAccountController := controller.NewServiceAccountController(s.wonderNetService)
	deployerController := controller.NewDeployerController(s.wonderNetService)

	headscaleProxy, err := controller.NewHeadscaleProxyController("http://127.0.0.1:8080")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /coordinator/health", healthController.ServeHTTP)

	// Worker endpoints (join token exchange doesn't require auth)
	mux.HandleFunc("POST /coordinator/api/v1/worker/join", workerController.HandleWorkerJoin)

	// Device flow endpoints (used by CLI for device authorization)
	mux.HandleFunc("POST /coordinator/device/code", deviceFlowController.HandleDeviceCode)
	mux.HandleFunc("GET /coordinator/device/verify", deviceFlowController.HandleDeviceVerifyPage)
	mux.HandleFunc("POST /coordinator/device/verify", s.requireAuth(s.requireWonderNet(deviceFlowController.HandleDeviceVerify)))
	mux.HandleFunc("POST /coordinator/device/token", deviceFlowController.HandleDeviceToken)

	// Protected endpoints - require JWT authentication and WonderNet
	mux.HandleFunc("POST /coordinator/api/v1/authkey", s.requireAuth(s.requireWonderNet(authKeyController.HandleCreateAuthKey)))
	mux.HandleFunc("POST /coordinator/api/v1/join-token", s.requireAuth(s.requireWonderNet(joinTokenController.HandleCreateJoinToken)))
	mux.HandleFunc("GET /coordinator/api/v1/nodes", s.requireAuth(s.requireWonderNet(nodesController.HandleListNodes)))
	mux.HandleFunc("POST /coordinator/api/v1/service-accounts", s.requireAuth(s.requireWonderNet(serviceAccountController.HandleCreate)))
	mux.HandleFunc("GET /coordinator/api/v1/service-accounts", s.requireAuth(s.requireWonderNet(serviceAccountController.HandleList)))
	mux.HandleFunc("DELETE /coordinator/api/v1/service-accounts/{id}", s.requireAuth(s.requireWonderNet(serviceAccountController.HandleDelete)))
	mux.HandleFunc("POST /coordinator/api/v1/deployer/join", s.requireAuth(s.requireWonderNet(deployerController.HandleDeployerJoin)))

	mux.HandleFunc("/coordinator/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.Handle("/", headscaleProxy)

	slog.Info("initializing ACL policy")
	ctx := context.Background()
	if err := s.wonderNetService.InitializeACLPolicy(ctx); err != nil {
		slog.Warn("initialize ACL policy", "error", err)
	} else {
		slog.Info("ACL policy initialized successfully")
	}

	httpServer := &http.Server{
		Addr:    s.config.Listen,
		Handler: mux,
	}

	go func() {
		slog.Info("starting coordinator",
			"listen", s.config.Listen,
			"coordinator_api", s.config.PublicURL+"/coordinator/*",
			"headscale", s.config.PublicURL+"/*",
			"keycloak", s.config.KeycloakURL)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := s.deviceFlowRepository.DeleteExpired(context.Background()); err != nil {
				slog.Warn("cleanup expired device requests", "error", err)
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		return err
	}

	return s.Close()
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
