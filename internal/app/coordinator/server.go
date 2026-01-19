package coordinator

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/controller"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/webui"
	"github.com/strrl/wonder-mesh-net/pkg/apikey"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
	"github.com/strrl/wonder-mesh-net/pkg/jwtauth"
	"github.com/strrl/wonder-mesh-net/pkg/meshbackend"
	"github.com/strrl/wonder-mesh-net/pkg/meshbackend/tailscale"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const minJWTSecretLength = 32

// Server is the coordinator server that manages multi-tenant wonder net access.
type Server struct {
	config          *Config
	db              *database.Manager
	headscaleConn   *grpc.ClientConn
	headscaleClient v1.HeadscaleServiceClient

	jwtValidator *jwtauth.Validator
	oidcService  *service.OIDCService

	meshBackend meshbackend.MeshBackend

	wonderNetRepository *repository.WonderNetRepository
	apiKeyRepository    *repository.APIKeyRepository

	wonderNetService *service.WonderNetService
	workerService    *service.WorkerService
	nodesService     *service.NodesService
	apiKeyService    *service.APIKeyService
}

// BootstrapNewServer creates a new coordinator server.
func BootstrapNewServer(config *Config) (*Server, error) {
	if len(config.JWTSecret) < minJWTSecretLength {
		return nil, fmt.Errorf("JWT secret must be at least %d bytes", minJWTSecretLength)
	}

	if err := os.MkdirAll(DefaultCoordinatorDataDir, 0755); err != nil {
		return nil, fmt.Errorf("create coordinator data dir: %w", err)
	}

	driver, err := database.ParseDriver(config.DatabaseDriver)
	if err != nil {
		return nil, fmt.Errorf("parse database driver: %w", err)
	}

	dsn := config.DatabaseDSN
	if dsn == "" {
		if driver == database.DriverSQLite {
			dsn = DefaultDatabaseDSN
		} else {
			return nil, fmt.Errorf("database DSN is required for driver %s", driver)
		}
	}

	db, err := database.NewManager(database.Config{
		Driver: driver,
		DSN:    dsn,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	slog.Info("database initialized", "driver", driver, "dsn", redactDSN(dsn))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	slog.Info("connecting to Headscale", "socket", config.HeadscaleUnixSocket)
	headscaleConn, err := grpc.NewClient(
		"unix://"+config.HeadscaleUnixSocket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to headscale: %w", err)
	}
	headscaleClient := v1.NewHeadscaleServiceClient(headscaleConn)

	// Create token generator for join tokens
	tokenGenerator := jointoken.NewGenerator(config.JWTSecret, config.PublicURL)

	// Create repositories
	wonderNetRepository := repository.NewWonderNetRepository(db.Queries())
	apiKeyRepository := repository.NewAPIKeyRepository(db.Queries())

	// Create Headscale managers
	wonderNetManager := headscale.NewWonderNetManager(headscaleClient)
	aclManager := headscale.NewACLManager(headscaleClient)

	// Create mesh backend (Tailscale via Headscale)
	meshBackend := tailscale.NewTailscaleMesh(headscaleClient, config.PublicURL)

	// Create services
	wonderNetService := service.NewWonderNetService(wonderNetRepository, wonderNetManager, aclManager, config.PublicURL)
	workerService := service.NewWorkerService(tokenGenerator, config.JWTSecret, wonderNetRepository, meshBackend)
	nodesService := service.NewNodesService(meshBackend)
	apiKeyService := service.NewAPIKeyService(apiKeyRepository, wonderNetRepository)

	// Create JWT validator for Keycloak tokens
	jwksURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/certs", config.KeycloakURL, config.KeycloakRealm)
	issuer := fmt.Sprintf("%s/realms/%s", config.KeycloakURL, config.KeycloakRealm)
	jwtValidator := jwtauth.NewValidator(jwtauth.ValidatorConfig{
		JWKSURL:         jwksURL,
		Issuer:          issuer,
		Audience:        config.KeycloakClientID,
		RefreshInterval: 5 * time.Minute,
	})

	if err := jwtValidator.Start(ctx); err != nil {
		_ = headscaleConn.Close()
		_ = db.Close()
		return nil, fmt.Errorf("start JWT validator: %w", err)
	}
	slog.Info("JWT validator started", "jwks_url", jwksURL)

	oidcService := service.NewOIDCService(service.OIDCConfig{
		KeycloakURL:  config.KeycloakURL,
		Realm:        config.KeycloakRealm,
		ClientID:     config.KeycloakClientID,
		ClientSecret: config.KeycloakClientSecret,
		RedirectURI:  config.PublicURL + "/coordinator/oidc/callback",
	}, jwtValidator)

	return &Server{
		config:              config,
		db:                  db,
		headscaleConn:       headscaleConn,
		headscaleClient:     headscaleClient,
		jwtValidator:        jwtValidator,
		oidcService:         oidcService,
		meshBackend:         meshBackend,
		wonderNetRepository: wonderNetRepository,
		apiKeyRepository:    apiKeyRepository,
		wonderNetService:    wonderNetService,
		workerService:       workerService,
		nodesService:        nodesService,
		apiKeyService:       apiKeyService,
	}, nil
}

func redactDSN(dsn string) string {
	// SQLite DSNs use "file:" prefix or plain paths, not URL format
	if strings.HasPrefix(dsn, "file:") || !strings.Contains(dsn, "://") {
		return "[sqlite]"
	}

	u, err := url.Parse(dsn)
	if err != nil {
		// If parsing fails, return a generic redacted indicator
		return "[redacted]"
	}
	// Go 1.15+ Redacted() method automatically hides passwords
	return u.Redacted()
}

// requireAuth wraps a handler with JWT authentication.
// It validates the JWT token (from Authorization header) or session cookie
// and adds the claims to the request context.
// This middleware only handles authentication, not WonderNet resolution.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token != "" {
			claims, err := s.jwtValidator.Validate(token)
			if err != nil {
				slog.Debug("JWT validation failed", "error", err)
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), jwtauth.ContextKeyClaims, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		cookie, err := r.Cookie(s.oidcService.GetSessionCookieName())
		if err == nil && cookie.Value != "" {
			session, err := s.oidcService.GetSession(cookie.Value)
			if err == nil {
				claims, err := s.jwtValidator.Validate(session.AccessToken)
				if err == nil {
					ctx := context.WithValue(r.Context(), jwtauth.ContextKeyClaims, claims)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				slog.Debug("session access token validation failed", "error", err)
			}
		}

		http.Error(w, "authorization required", http.StatusUnauthorized)
	}
}

// requireWonderNet wraps a handler to resolve the WonderNet from JWT claims.
// For regular users, it auto-creates a WonderNet if none exists.
// Must be used after requireAuth.
func (s *Server) requireWonderNet(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := jwtauth.ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		wonderNet, err := s.wonderNetService.ResolveWonderNetFromClaims(r.Context(), claims)
		if err != nil {
			slog.Error("get or create wonder net", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		ctx := context.WithValue(r.Context(), controller.ContextKeyWonderNet, wonderNet)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// requireAPIKey wraps a handler with API key authentication.
// It validates the API key and adds the associated WonderNet to the context.
func (s *Server) requireAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		if !apikey.IsAPIKey(token) {
			http.Error(w, "invalid api key format", http.StatusUnauthorized)
			return
		}

		wonderNet, err := s.apiKeyService.ValidateAPIKey(r.Context(), token)
		if err != nil {
			slog.Debug("API key validation failed", "error", err)
			http.Error(w, "invalid api key", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), controller.ContextKeyWonderNet, wonderNet)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// requireAuthOrAPIKey wraps a handler that accepts JWT session auth, session cookie, or API key auth.
// For JWT/session auth, it validates the token and resolves the WonderNet from claims.
// For API key auth, it validates the key and uses the associated WonderNet.
// This is used for read-only endpoints that should be accessible to both users and third-party integrations.
func (s *Server) requireAuthOrAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)

		// Check if it's an API key
		if token != "" && apikey.IsAPIKey(token) {
			wonderNet, err := s.apiKeyService.ValidateAPIKey(r.Context(), token)
			if err != nil {
				slog.Debug("API key validation failed", "error", err)
				http.Error(w, "invalid api key", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), controller.ContextKeyWonderNet, wonderNet)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Try JWT from Authorization header
		if token != "" {
			claims, err := s.jwtValidator.Validate(token)
			if err != nil {
				slog.Debug("JWT validation failed", "error", err)
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			wonderNet, err := s.wonderNetService.ResolveWonderNetFromClaims(r.Context(), claims)
			if err != nil {
				slog.Error("resolve wonder net from claims", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}

			ctx := context.WithValue(r.Context(), controller.ContextKeyWonderNet, wonderNet)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Try session cookie
		cookie, err := r.Cookie(s.oidcService.GetSessionCookieName())
		if err == nil && cookie.Value != "" {
			session, err := s.oidcService.GetSession(cookie.Value)
			if err == nil {
				claims, err := s.jwtValidator.Validate(session.AccessToken)
				if err == nil {
					wonderNet, err := s.wonderNetService.ResolveWonderNetFromClaims(r.Context(), claims)
					if err == nil {
						ctx := context.WithValue(r.Context(), controller.ContextKeyWonderNet, wonderNet)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
					slog.Error("resolve wonder net from claims", "error", err)
				}
				slog.Debug("session access token validation failed", "error", err)
			}
		}

		http.Error(w, "authorization required", http.StatusUnauthorized)
	}
}

// requireAdminAuth wraps a handler with admin API authentication.
// It validates the bearer token against the configured AdminAPIAuthToken using constant-time comparison.
func (s *Server) requireAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		if subtle.ConstantTimeCompare([]byte(token), []byte(s.config.AdminAPIAuthToken)) != 1 {
			http.Error(w, "invalid admin token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
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
	joinTokenController := controller.NewJoinTokenController(s.workerService)
	nodesController := controller.NewNodesController(s.nodesService)
	apiKeyController := controller.NewAPIKeyController(s.apiKeyService)
	deployerController := controller.NewDeployerController(s.meshBackend)

	secureCookie := strings.HasPrefix(s.config.PublicURL, "https://")
	oidcController := controller.NewOIDCController(
		s.oidcService,
		s.wonderNetService,
		s.config.PublicURL,
		secureCookie,
	)

	headscaleProxy, err := controller.NewHeadscaleProxyController(s.config.HeadscaleURL)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /coordinator/health", healthController.ServeHTTP)

	// OIDC authentication endpoints (no auth required)
	mux.HandleFunc("GET /coordinator/oidc/login", oidcController.HandleLogin)
	mux.HandleFunc("GET /coordinator/oidc/callback", oidcController.HandleCallback)
	mux.HandleFunc("GET /coordinator/oidc/logout", oidcController.HandleLogout)

	// Worker endpoints (join token exchange doesn't require auth)
	mux.HandleFunc("POST /coordinator/api/v1/worker/join", workerController.HandleWorkerJoin)

	// Protected endpoints - require JWT authentication and WonderNet
	mux.HandleFunc("GET /coordinator/api/v1/join-token", s.requireAuth(s.requireWonderNet(joinTokenController.HandleCreateJoinToken)))

	// Read-only endpoints - support both JWT session auth and API key auth
	mux.HandleFunc("GET /coordinator/api/v1/nodes", s.requireAuthOrAPIKey(nodesController.HandleListNodes))

	// API key management - JWT auth only (no API key auth to prevent privilege escalation)
	mux.HandleFunc("POST /coordinator/api/v1/api-keys", s.requireAuth(s.requireWonderNet(apiKeyController.HandleCreate)))
	mux.HandleFunc("GET /coordinator/api/v1/api-keys", s.requireAuth(s.requireWonderNet(apiKeyController.HandleList)))
	mux.HandleFunc("DELETE /coordinator/api/v1/api-keys/{id}", s.requireAuth(s.requireWonderNet(apiKeyController.HandleDelete)))

	// Deployer endpoints - API key auth only
	mux.HandleFunc("POST /coordinator/api/v1/deployer/join", s.requireAPIKey(deployerController.HandleDeployerJoin))

	// Admin API endpoints - only registered if enabled
	if s.config.EnableAdminAPI {
		adminController := controller.NewAdminController(
			s.wonderNetService,
			s.nodesService,
			s.workerService,
			s.apiKeyService,
			s.meshBackend,
		)
		mux.HandleFunc("GET /coordinator/admin/api/v1/wonder-nets", s.requireAdminAuth(adminController.HandleListWonderNets))
		mux.HandleFunc("POST /coordinator/admin/api/v1/wonder-nets", s.requireAdminAuth(adminController.HandleAdminCreateWonderNet))
		mux.HandleFunc("GET /coordinator/admin/api/v1/wonder-nets/{id}/nodes", s.requireAdminAuth(adminController.HandleListWonderNetNodes))
		mux.HandleFunc("GET /coordinator/admin/api/v1/users/{user_id}/wonder-nets", s.requireAdminAuth(adminController.HandleListWonderNetsByUser))
		mux.HandleFunc("GET /coordinator/admin/api/v1/nodes", s.requireAdminAuth(adminController.HandleListAllNodes))
		mux.HandleFunc("POST /coordinator/admin/api/v1/wonder-nets/{id}/join-token", s.requireAdminAuth(adminController.HandleAdminCreateJoinToken))
		mux.HandleFunc("POST /coordinator/admin/api/v1/wonder-nets/{id}/api-keys", s.requireAdminAuth(adminController.HandleAdminCreateAPIKey))
		mux.HandleFunc("POST /coordinator/admin/api/v1/wonder-nets/{id}/deployer/join", s.requireAdminAuth(adminController.HandleAdminDeployerJoin))
		slog.Info("admin API routes registered")
	}

	mux.HandleFunc("/coordinator/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	// Web UI - served at /ui/
	uiHandler, err := webui.Handler()
	if err != nil {
		return fmt.Errorf("initialize ui handler: %w", err)
	}
	mux.Handle("/ui/", http.StripPrefix("/ui", uiHandler))

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

func (s *Server) Close() error {
	if s.headscaleConn != nil {
		_ = s.headscaleConn.Close()
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
