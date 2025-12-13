package coordinator

import (
	"context"
	"log"
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

	log.Println("Initializing ACL policy...")
	ctx := context.Background()
	if err := s.ACLManager.SetAutogroupSelfPolicy(ctx); err != nil {
		log.Printf("Warning: failed to initialize ACL policy: %v", err)
	} else {
		log.Println("ACL policy initialized successfully")
	}

	httpServer := &http.Server{
		Addr:    s.Config.Listen,
		Handler: rootHandler,
	}

	go func() {
		log.Printf("Starting coordinator on %s", s.Config.Listen)
		log.Printf("  Coordinator API: %s/coordinator/*", s.Config.PublicURL)
		log.Printf("  Headscale:       %s/*", s.Config.PublicURL)
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
