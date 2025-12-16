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
	"github.com/strrl/wonder-mesh-net/app/coordinator/handlers"
)

// Run starts the HTTP server and blocks until a shutdown signal is received.
// It registers all API routes, starts listening on the configured address,
// and handles graceful shutdown on SIGINT or SIGTERM with a 10-second timeout.
func (s *Server) Run() error {
	healthHandler := handlers.NewHealthHandler(s.HSClient)
	authHelper := handlers.NewAuthHelper(s.SessionStore, s.UserStore)
	authHandler := handlers.NewAuthHandler(
		s.Config.PublicURL,
		s.OIDCRegistry,
		s.RealmManager,
		s.ACLManager,
		s.SessionStore,
		s.UserStore,
	)
	nodesHandler := handlers.NewNodesHandler(s.RealmManager, s.APIKeyStore, authHelper)
	apiKeyHandler := handlers.NewAPIKeyHandler(s.APIKeyStore, authHelper)
	deployerHandler := handlers.NewDeployerHandler(s.Config.PublicURL, s.RealmManager, s.APIKeyStore, authHelper)
	workerHandler := handlers.NewWorkerHandler(
		s.Config.PublicURL,
		s.Config.JWTSecret,
		s.RealmManager,
		s.TokenGenerator,
		s.SessionStore,
		s.UserStore,
	)

	hsProxy, err := handlers.NewHeadscaleProxyHandler("http://127.0.0.1:8080")
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

	rootRouter := chi.NewRouter()
	rootRouter.Mount("/coordinator", coordinatorRouter)
	rootRouter.NotFound(hsProxy.ServeHTTP)

	slog.Info("initializing ACL policy")
	ctx := context.Background()
	if err := s.ACLManager.SetAutogroupSelfPolicy(ctx); err != nil {
		slog.Warn("failed to initialize ACL policy", "error", err)
	} else {
		slog.Info("ACL policy initialized successfully")
	}

	httpServer := &http.Server{
		Addr:    s.Config.Listen,
		Handler: rootRouter,
	}

	go func() {
		slog.Info("starting coordinator",
			"listen", s.Config.Listen,
			"coordinator_api", s.Config.PublicURL+"/coordinator/*",
			"headscale", s.Config.PublicURL+"/*")
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
