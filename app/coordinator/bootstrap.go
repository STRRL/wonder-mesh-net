package coordinator

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/strrl/wonder-mesh-net/app/coordinator/handlers"
)

// Run starts the HTTP server and blocks until a shutdown signal is received.
// It registers all API routes, starts listening on the configured address,
// and handles graceful shutdown on SIGINT or SIGTERM with a 10-second timeout.
func (s *Server) Run() error {
	healthHandler := handlers.NewHealthHandler(s.HSClient)
	authHandler := handlers.NewAuthHandler(
		s.Config.PublicURL,
		s.OIDCRegistry,
		s.TenantManager,
		s.ACLManager,
		s.SessionStore,
		s.UserStore,
	)
	nodesHandler := handlers.NewNodesHandler(s.TenantManager, s.SessionStore, s.UserStore)
	workerHandler := handlers.NewWorkerHandler(
		s.Config.PublicURL,
		s.Config.JWTSecret,
		s.TenantManager,
		s.TokenGenerator,
		s.SessionStore,
		s.UserStore,
	)
	hsProxyHandler, err := handlers.NewHeadscaleProxyHandler("http://127.0.0.1:8080", "/hs")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/health", healthHandler)
	mux.HandleFunc("/auth/providers", authHandler.HandleProviders)
	mux.HandleFunc("/auth/login", authHandler.HandleLogin)
	mux.HandleFunc("/auth/callback", authHandler.HandleCallback)
	mux.HandleFunc("/auth/complete", authHandler.HandleComplete)
	mux.HandleFunc("/api/v1/authkey", authHandler.HandleCreateAuthKey)
	mux.HandleFunc("/api/v1/nodes", nodesHandler.HandleListNodes)
	mux.HandleFunc("/api/v1/join-token", workerHandler.HandleCreateJoinToken)
	mux.HandleFunc("/api/v1/worker/join", workerHandler.HandleWorkerJoin)
	mux.Handle("/hs/", hsProxyHandler)

	httpServer := &http.Server{
		Addr:    s.Config.ListenAddr,
		Handler: mux,
	}

	go func() {
		log.Printf("Starting coordinator on %s", s.Config.ListenAddr)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		return err
	}

	return s.Close()
}
