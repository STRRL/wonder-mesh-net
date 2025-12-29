package coordinator

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/controller"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/jwtauth"
)

// requireAuth wraps a handler with JWT authentication.
// It validates the JWT token and adds the user's wonder net to the request context.
// Supports both regular users and service accounts.
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

		var wonderNet *repository.WonderNet

		// Service accounts have preferred_username starting with "service-account-"
		if strings.HasPrefix(claims.PreferredUsername, "service-account-") {
			wonderNet, err = s.keycloakAuthService.GetServiceAccountWonderNet(r.Context(), claims)
			if err != nil {
				slog.Error("get service account wonder net", "error", err)
				http.Error(w, "service account not associated with wonder net", http.StatusUnauthorized)
				return
			}
		} else {
			_, wonderNet, err = s.keycloakAuthService.EnsureUserAndWonderNet(r.Context(), claims)
			if err != nil {
				slog.Error("ensure user and wonder net", "error", err)
				http.Error(w, "authentication failed", http.StatusInternalServerError)
				return
			}
		}

		ctx := context.WithValue(r.Context(), controller.ContextKeyWonderNet, wonderNet)
		ctx = context.WithValue(ctx, jwtauth.ContextKeyClaims, claims)
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
	serviceAccountController := controller.NewServiceAccountController(s.keycloakAuthService)
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
	mux.HandleFunc("POST /coordinator/device/verify", s.requireAuth(deviceFlowController.HandleDeviceVerify))
	mux.HandleFunc("POST /coordinator/device/token", deviceFlowController.HandleDeviceToken)

	// Protected endpoints - require JWT authentication
	mux.HandleFunc("POST /coordinator/api/v1/authkey", s.requireAuth(authKeyController.HandleCreateAuthKey))
	mux.HandleFunc("POST /coordinator/api/v1/join-token", s.requireAuth(joinTokenController.HandleCreateJoinToken))
	mux.HandleFunc("GET /coordinator/api/v1/nodes", s.requireAuth(nodesController.HandleListNodes))
	mux.HandleFunc("POST /coordinator/api/v1/service-accounts", s.requireAuth(serviceAccountController.HandleCreate))
	mux.HandleFunc("GET /coordinator/api/v1/service-accounts", s.requireAuth(serviceAccountController.HandleList))
	mux.HandleFunc("DELETE /coordinator/api/v1/service-accounts/{id}", s.requireAuth(serviceAccountController.HandleDelete))
	mux.HandleFunc("POST /coordinator/api/v1/deployer/join", s.requireAuth(deployerController.HandleDeployerJoin))

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
