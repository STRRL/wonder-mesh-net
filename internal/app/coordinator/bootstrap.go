package coordinator

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/controller"
)

// Run starts the HTTP server and blocks until a shutdown signal is received.
// It registers all API routes, starts listening on the configured address,
// and handles graceful shutdown on SIGINT or SIGTERM with a 10-second timeout.
func (s *Server) Run() error {
	healthController := controller.NewHealthController(s.headscaleClient)
	oidcController := controller.NewOIDCController(s.oidcService, s.authService, s.realmService, s.config.PublicURL)
	nodesController := controller.NewNodesController(s.nodesService, s.authService)
	apiKeyController := controller.NewAPIKeyController(s.apiKeyRepository, s.authService)
	deployerController := controller.NewDeployerController(s.realmService, s.authService)
	workerController := controller.NewWorkerController(s.workerService, s.authService)
	deviceController := controller.NewDeviceController(s.deviceFlowService, s.authService, s.config.PublicURL)

	headscaleProxy, err := controller.NewHeadscaleProxyController("http://127.0.0.1:8080")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /coordinator/health", healthController.ServeHTTP)
	mux.HandleFunc("GET /coordinator/oidc/providers", oidcController.HandleProviders)
	mux.HandleFunc("GET /coordinator/oidc/login", oidcController.HandleLogin)
	mux.HandleFunc("GET /coordinator/oidc/callback", oidcController.HandleCallback)
	mux.HandleFunc("GET /coordinator/oidc/complete", oidcController.HandleComplete)
	mux.HandleFunc("POST /coordinator/api/v1/authkey", oidcController.HandleCreateAuthKey)
	mux.HandleFunc("GET /coordinator/api/v1/nodes", nodesController.HandleListNodes)
	mux.HandleFunc("GET /coordinator/api/v1/api-keys", apiKeyController.HandleListAPIKeys)
	mux.HandleFunc("POST /coordinator/api/v1/api-keys", apiKeyController.HandleCreateAPIKey)
	mux.HandleFunc("DELETE /coordinator/api/v1/api-keys/{id}", apiKeyController.HandleDeleteAPIKey)
	mux.HandleFunc("POST /coordinator/api/v1/join-token", workerController.HandleCreateJoinToken)
	mux.HandleFunc("POST /coordinator/api/v1/worker/join", workerController.HandleWorkerJoin)
	mux.HandleFunc("POST /coordinator/api/v1/deployer/join", deployerController.HandleDeployerJoin)
	mux.HandleFunc("POST /coordinator/device/code", deviceController.HandleDeviceCode)
	mux.HandleFunc("GET /coordinator/device/verify", deviceController.HandleDeviceVerifyPage)
	mux.HandleFunc("POST /coordinator/device/verify", deviceController.HandleDeviceVerify)
	mux.HandleFunc("POST /coordinator/device/token", deviceController.HandleDeviceToken)
	mux.Handle("/", headscaleProxy)

	slog.Info("initializing ACL policy")
	ctx := context.Background()
	if err := s.realmService.InitializeACLPolicy(ctx); err != nil {
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
			"headscale", s.config.PublicURL+"/*")
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
