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

	"github.com/strrl/wonder-mesh-net/app/coordinator/handlers"
)

const coordinatorPrefix = "/coordinator"

// Run starts the HTTP server and blocks until a shutdown signal is received.
// It registers all API routes, starts listening on the configured address,
// and handles graceful shutdown on SIGINT or SIGTERM with a 10-second timeout.
func (s *Server) Run() error {
	healthHandler := handlers.NewHealthHandler(s.HSClient)
	authHandler := handlers.NewAuthHandler(
		s.Config.PublicURL,
		s.OIDCRegistry,
		s.RealmManager,
		s.ACLManager,
		s.SessionStore,
		s.UserStore,
	)
	nodesHandler := handlers.NewNodesHandler(s.RealmManager, s.SessionStore, s.UserStore, s.APIKeyStore)
	apiKeyHandler := handlers.NewAPIKeyHandler(s.APIKeyStore, s.SessionStore, s.UserStore)
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

	coordinatorMux := http.NewServeMux()
	coordinatorMux.Handle("/livez", &handlers.LivenessHandler{})
	coordinatorMux.Handle("/health", healthHandler)
	coordinatorMux.HandleFunc("/auth/providers", authHandler.HandleProviders)
	coordinatorMux.HandleFunc("/auth/login", authHandler.HandleLogin)
	coordinatorMux.HandleFunc("/auth/callback", authHandler.HandleCallback)
	coordinatorMux.HandleFunc("/auth/complete", authHandler.HandleComplete)
	coordinatorMux.HandleFunc("/api/v1/authkey", authHandler.HandleCreateAuthKey)
	coordinatorMux.HandleFunc("/api/v1/nodes", nodesHandler.HandleListNodes)
	coordinatorMux.HandleFunc("/api/v1/api-keys", apiKeyHandler.HandleAPIKeys)
	coordinatorMux.HandleFunc("/api/v1/api-keys/", apiKeyHandler.HandleDeleteAPIKey)
	coordinatorMux.HandleFunc("/api/v1/join-token", workerHandler.HandleCreateJoinToken)
	coordinatorMux.HandleFunc("/api/v1/worker/join", workerHandler.HandleWorkerJoin)

	rootHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, coordinatorPrefix+"/") {
			http.StripPrefix(coordinatorPrefix, coordinatorMux).ServeHTTP(w, r)
			return
		}
		if r.URL.Path == coordinatorPrefix {
			http.StripPrefix(coordinatorPrefix, coordinatorMux).ServeHTTP(w, r)
			return
		}
		hsProxy.ServeHTTP(w, r)
	})

	slog.Info("initializing ACL policy")
	ctx := context.Background()
	if err := s.ACLManager.SetAutogroupSelfPolicy(ctx); err != nil {
		slog.Warn("failed to initialize ACL policy", "error", err)
	} else {
		slog.Info("ACL policy initialized successfully")
	}

	httpServer := &http.Server{
		Addr:    s.Config.Listen,
		Handler: rootHandler,
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
