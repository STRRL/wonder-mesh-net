package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

var coordinatorCmd = &cobra.Command{
	Use:   "coordinator",
	Short: "Run the coordinator server",
	Long:  `Run the Wonder Mesh Net coordinator server that wraps Headscale API and provides OIDC authentication.`,
	Run:   runCoordinator,
}

func init() {
	coordinatorCmd.Flags().String("listen", ":8080", "Listen address")
	coordinatorCmd.Flags().String("headscale-url", "http://localhost:8080", "Headscale API URL")
	coordinatorCmd.Flags().String("headscale-api-key", "", "Headscale API key")
	coordinatorCmd.Flags().String("public-url", "http://localhost:8080", "Public URL for callbacks")

	_ = viper.BindPFlag("coordinator.listen", coordinatorCmd.Flags().Lookup("listen"))
	_ = viper.BindPFlag("coordinator.headscale_url", coordinatorCmd.Flags().Lookup("headscale-url"))
	_ = viper.BindPFlag("coordinator.headscale_api_key", coordinatorCmd.Flags().Lookup("headscale-api-key"))
	_ = viper.BindPFlag("coordinator.public_url", coordinatorCmd.Flags().Lookup("public-url"))

	_ = viper.BindEnv("coordinator.headscale_api_key", "HEADSCALE_API_KEY")
	_ = viper.BindEnv("coordinator.jwt_secret", "JWT_SECRET")
	_ = viper.BindEnv("coordinator.github_client_id", "GITHUB_CLIENT_ID")
	_ = viper.BindEnv("coordinator.github_client_secret", "GITHUB_CLIENT_SECRET")
	_ = viper.BindEnv("coordinator.google_client_id", "GOOGLE_CLIENT_ID")
	_ = viper.BindEnv("coordinator.google_client_secret", "GOOGLE_CLIENT_SECRET")
}

type coordinatorConfig struct {
	ListenAddr      string
	HeadscaleURL    string
	HeadscaleAPIKey string
	PublicURL       string
	JWTSecret       string
	OIDCProviders   []oidc.ProviderConfig
}

type coordinatorServer struct {
	config         *coordinatorConfig
	hsClient       *headscale.Client
	tenantManager  *headscale.TenantManager
	aclManager     *headscale.ACLManager
	oidcRegistry   *oidc.Registry
	tokenGenerator *jointoken.Generator
}

func newCoordinatorServer(config *coordinatorConfig) (*coordinatorServer, error) {
	hsClient := headscale.NewClient(config.HeadscaleURL, config.HeadscaleAPIKey)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := hsClient.Health(ctx); err != nil {
		return nil, fmt.Errorf("headscale health check failed: %w", err)
	}

	oidcRegistry := oidc.NewRegistry()
	for _, providerConfig := range config.OIDCProviders {
		if err := oidcRegistry.RegisterProvider(ctx, providerConfig); err != nil {
			log.Printf("Warning: failed to register OIDC provider %s: %v", providerConfig.Name, err)
		} else {
			log.Printf("Registered OIDC provider: %s", providerConfig.Name)
		}
	}

	tokenGenerator := jointoken.NewGenerator(
		config.JWTSecret,
		config.PublicURL,
		config.HeadscaleURL,
	)

	return &coordinatorServer{
		config:         config,
		hsClient:       hsClient,
		tenantManager:  headscale.NewTenantManager(hsClient),
		aclManager:     headscale.NewACLManager(hsClient),
		oidcRegistry:   oidcRegistry,
		tokenGenerator: tokenGenerator,
	}, nil
}

func (s *coordinatorServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.hsClient.Health(ctx); err != nil {
		http.Error(w, "headscale unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintln(w, "ok")
}

func (s *coordinatorServer) handleProviders(w http.ResponseWriter, r *http.Request) {
	providers := s.oidcRegistry.ListProviders()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"providers": providers,
	})
}

func (s *coordinatorServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		http.Error(w, "provider parameter required", http.StatusBadRequest)
		return
	}

	provider, ok := s.oidcRegistry.GetProvider(providerName)
	if !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}

	cliRedirect := r.URL.Query().Get("redirect_uri")
	if cliRedirect == "" {
		http.Error(w, "redirect_uri parameter required", http.StatusBadRequest)
		return
	}

	authState, err := s.oidcRegistry.CreateAuthState(cliRedirect)
	if err != nil {
		http.Error(w, "failed to create auth state", http.StatusInternalServerError)
		return
	}

	callbackURL := s.config.PublicURL + "/auth/callback?provider=" + providerName
	authURL := provider.GetAuthURL(callbackURL, authState.State)

	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *coordinatorServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	providerName := r.URL.Query().Get("provider")
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if providerName == "" || code == "" || state == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	authState, ok := s.oidcRegistry.ValidateState(state)
	if !ok {
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	provider, ok := s.oidcRegistry.GetProvider(providerName)
	if !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}

	callbackURL := s.config.PublicURL + "/auth/callback?provider=" + providerName
	userInfo, err := provider.ExchangeCode(ctx, code, callbackURL)
	if err != nil {
		log.Printf("Failed to exchange code: %v", err)
		http.Error(w, "failed to exchange code", http.StatusInternalServerError)
		return
	}

	user, err := s.tenantManager.GetOrCreateTenant(ctx, provider.Issuer(), userInfo.Subject)
	if err != nil {
		log.Printf("Failed to get/create tenant: %v", err)
		http.Error(w, "failed to create tenant", http.StatusInternalServerError)
		return
	}

	if err := s.aclManager.AddTenantToPolicy(ctx, user.Name); err != nil {
		log.Printf("Warning: failed to update ACL policy: %v", err)
	}

	sessionToken := headscale.DeriveTenantID(provider.Issuer(), userInfo.Subject)

	redirectURL := authState.RedirectURI + "?session=" + sessionToken + "&user=" + user.Name
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (s *coordinatorServer) handleCreateAuthKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	session := r.Header.Get("X-Session-Token")
	if session == "" {
		http.Error(w, "session token required", http.StatusUnauthorized)
		return
	}

	userName := "tenant-" + session[:12]
	user, err := s.hsClient.GetUser(ctx, userName)
	if err != nil || user == nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	var req struct {
		TTL      string `json:"ttl"`
		Reusable bool   `json:"reusable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ttl := 24 * time.Hour
	if req.TTL != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			http.Error(w, "invalid TTL format", http.StatusBadRequest)
			return
		}
		ttl = parsed
	}

	key, err := s.tenantManager.CreateAuthKey(ctx, user.ID, ttl, req.Reusable)
	if err != nil {
		log.Printf("Failed to create auth key: %v", err)
		http.Error(w, "failed to create auth key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"key":        key.Key,
		"expiration": key.Expiration,
		"reusable":   key.Reusable,
	})
}

func (s *coordinatorServer) handleListNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := r.Header.Get("X-Session-Token")
	if session == "" {
		http.Error(w, "session token required", http.StatusUnauthorized)
		return
	}

	userName := "tenant-" + session[:12]
	user, err := s.hsClient.GetUser(ctx, userName)
	if err != nil || user == nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	nodes, err := s.tenantManager.GetTenantNodes(ctx, user.ID)
	if err != nil {
		log.Printf("Failed to list nodes: %v", err)
		http.Error(w, "failed to list nodes", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
	})
}

func (s *coordinatorServer) handleCreateJoinToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	session := r.Header.Get("X-Session-Token")
	if session == "" {
		http.Error(w, "session token required", http.StatusUnauthorized)
		return
	}

	userName := "tenant-" + session[:12]
	user, err := s.hsClient.GetUser(ctx, userName)
	if err != nil || user == nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	var req struct {
		TTL string `json:"ttl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.TTL = "1h"
	}

	ttl := time.Hour
	if req.TTL != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			http.Error(w, "invalid TTL format", http.StatusBadRequest)
			return
		}
		ttl = parsed
	}

	token, err := s.tokenGenerator.Generate(session, userName, ttl)
	if err != nil {
		log.Printf("Failed to generate join token: %v", err)
		http.Error(w, "failed to generate join token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"token":   token,
		"command": fmt.Sprintf("wonder worker join %s", token),
	})
}

func (s *coordinatorServer) handleWorkerJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	validator := jointoken.NewValidator(s.config.JWTSecret)
	claims, err := validator.Validate(req.Token)
	if err != nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	userName := "tenant-" + claims.Session[:12]
	user, err := s.hsClient.GetUser(ctx, userName)
	if err != nil || user == nil {
		http.Error(w, "invalid session in token", http.StatusUnauthorized)
		return
	}

	key, err := s.tenantManager.CreateAuthKey(ctx, user.ID, 24*time.Hour, false)
	if err != nil {
		log.Printf("Failed to create auth key: %v", err)
		http.Error(w, "failed to create auth key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"authkey":       key.Key,
		"headscale_url": s.config.HeadscaleURL,
		"user":          userName,
	})
}

func runCoordinator(cmd *cobra.Command, args []string) {
	listenAddr := viper.GetString("coordinator.listen")
	headscaleURL := viper.GetString("coordinator.headscale_url")
	headscaleAPIKey := viper.GetString("coordinator.headscale_api_key")
	publicURL := viper.GetString("coordinator.public_url")

	if headscaleAPIKey == "" {
		log.Fatal("headscale-api-key is required (flag, config, or HEADSCALE_API_KEY env)")
	}

	jwtSecret := viper.GetString("coordinator.jwt_secret")
	if jwtSecret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			log.Fatalf("Failed to generate JWT secret: %v", err)
		}
		jwtSecret = hex.EncodeToString(b)
		log.Printf("Warning: JWT_SECRET not set, generated random secret (tokens won't survive restart)")
	}

	config := &coordinatorConfig{
		ListenAddr:      listenAddr,
		HeadscaleURL:    headscaleURL,
		HeadscaleAPIKey: headscaleAPIKey,
		PublicURL:       publicURL,
		JWTSecret:       jwtSecret,
		OIDCProviders:   []oidc.ProviderConfig{},
	}

	if githubClientID := viper.GetString("coordinator.github_client_id"); githubClientID != "" {
		config.OIDCProviders = append(config.OIDCProviders, oidc.ProviderConfig{
			Type:         "github",
			Name:         "github",
			ClientID:     githubClientID,
			ClientSecret: viper.GetString("coordinator.github_client_secret"),
		})
	}

	if googleClientID := viper.GetString("coordinator.google_client_id"); googleClientID != "" {
		config.OIDCProviders = append(config.OIDCProviders, oidc.ProviderConfig{
			Type:         "google",
			Name:         "google",
			ClientID:     googleClientID,
			ClientSecret: viper.GetString("coordinator.google_client_secret"),
		})
	}

	server, err := newCoordinatorServer(config)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/auth/providers", server.handleProviders)
	mux.HandleFunc("/auth/login", server.handleLogin)
	mux.HandleFunc("/auth/callback", server.handleCallback)
	mux.HandleFunc("/api/v1/authkey", server.handleCreateAuthKey)
	mux.HandleFunc("/api/v1/nodes", server.handleListNodes)
	mux.HandleFunc("/api/v1/join-token", server.handleCreateJoinToken)
	mux.HandleFunc("/api/v1/worker/join", server.handleWorkerJoin)

	httpServer := &http.Server{
		Addr:    config.ListenAddr,
		Handler: mux,
	}

	go func() {
		log.Printf("Starting coordinator on %s", config.ListenAddr)
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
		log.Printf("Shutdown error: %v", err)
	}
}
