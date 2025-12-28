package coordinator

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/handlers"
)

// Run starts the HTTP server and blocks until a shutdown signal is received.
// It registers all API routes, starts listening on the configured address,
// and handles graceful shutdown on SIGINT or SIGTERM with a 10-second timeout.
func (s *Server) Run() error {
	healthHandler := handlers.NewHealthHandler(s.headscaleClient)
	authHelper := handlers.NewAuthHelper(s.sessionRepository, s.realmRepository)
	authHandler := handlers.NewAuthHandler(
		s.config.PublicURL,
		s.oidcRegistry,
		s.realmManager,
		s.aclManager,
		s.sessionRepository,
		s.userRepository,
		s.identityRepository,
		s.realmRepository,
	)
	nodesHandler := handlers.NewNodesHandler(s.realmManager, s.apiKeyRepository, authHelper)
	apiKeyHandler := handlers.NewAPIKeyHandler(s.apiKeyRepository, authHelper)
	deployerHandler := handlers.NewDeployerHandler(s.config.PublicURL, s.realmManager, s.apiKeyRepository, authHelper)
	workerHandler := handlers.NewWorkerHandler(
		s.config.PublicURL,
		s.config.JWTSecret,
		s.realmManager,
		s.tokenGenerator,
		s.sessionRepository,
		s.realmRepository,
	)
	deviceHandler := handlers.NewDeviceHandler(
		s.config.PublicURL,
		s.deviceFlowRepository,
		s.realmManager,
		authHelper,
	)

	headscaleProxy, err := handlers.NewHeadscaleProxyHandler("http://127.0.0.1:8080")
	if err != nil {
		return err
	}

	coordinatorRouter := chi.NewRouter()
	coordinatorRouter.Get("/livez", handlers.HandleLiveness)
	coordinatorRouter.Get("/health", healthHandler.ServeHTTP)
	coordinatorRouter.Get("/auth/providers", authHandler.HandleProviders)
	coordinatorRouter.Get("/auth/login", authHandler.HandleLogin)
	coordinatorRouter.Get("/auth/callback", authHandler.HandleCallback)
	coordinatorRouter.Get("/auth/complete", authHandler.HandleComplete)
	coordinatorRouter.Post("/api/v1/authkey", authHandler.HandleCreateAuthKey)
	coordinatorRouter.Get("/api/v1/nodes", nodesHandler.HandleListNodes)
	coordinatorRouter.Get("/api/v1/api-keys", apiKeyHandler.HandleListAPIKeys)
	coordinatorRouter.Post("/api/v1/api-keys", apiKeyHandler.HandleCreateAPIKey)
	coordinatorRouter.Delete("/api/v1/api-keys/{id}", apiKeyHandler.HandleDeleteAPIKey)
	coordinatorRouter.Post("/api/v1/join-token", workerHandler.HandleCreateJoinToken)
	coordinatorRouter.Post("/api/v1/worker/join", workerHandler.HandleWorkerJoin)
	coordinatorRouter.Post("/api/v1/deployer/join", deployerHandler.HandleDeployerJoin)
	coordinatorRouter.Post("/device/code", deviceHandler.HandleDeviceCode)
	coordinatorRouter.Get("/device/verify", deviceHandler.HandleDeviceVerifyPage)
	coordinatorRouter.Post("/device/verify", deviceHandler.HandleDeviceVerify)
	coordinatorRouter.Post("/device/token", deviceHandler.HandleDeviceToken)

	rootRouter := chi.NewRouter()
	rootRouter.Mount("/coordinator", coordinatorRouter)
	rootRouter.NotFound(headscaleProxy.ServeHTTP)

	slog.Info("initializing ACL policy")
	ctx := context.Background()
	if err := s.aclManager.SetAutogroupSelfPolicy(ctx); err != nil {
		slog.Warn("initialize ACL policy", "error", err)
	} else {
		slog.Info("ACL policy initialized successfully")
	}

	httpServer := &http.Server{
		Addr:    s.config.Listen,
		Handler: rootRouter,
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
