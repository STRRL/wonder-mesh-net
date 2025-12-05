package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
)

// WorkerHandler handles worker-related requests.
type WorkerHandler struct {
	headscaleURL       string
	headscalePublicURL string
	jwtSecret          string
	hsClient           *headscale.Client
	tenantManager      *headscale.TenantManager
	tokenGenerator     *jointoken.Generator
}

// NewWorkerHandler creates a new WorkerHandler.
func NewWorkerHandler(
	headscaleURL string,
	headscalePublicURL string,
	jwtSecret string,
	hsClient *headscale.Client,
	tenantManager *headscale.TenantManager,
	tokenGenerator *jointoken.Generator,
) *WorkerHandler {
	return &WorkerHandler{
		headscaleURL:       headscaleURL,
		headscalePublicURL: headscalePublicURL,
		jwtSecret:          jwtSecret,
		hsClient:           hsClient,
		tenantManager:      tenantManager,
		tokenGenerator:     tokenGenerator,
	}
}

// HandleCreateJoinToken handles POST /api/v1/join-token requests.
func (h *WorkerHandler) HandleCreateJoinToken(w http.ResponseWriter, r *http.Request) {
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
	user, err := h.hsClient.GetUser(ctx, userName)
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

	token, err := h.tokenGenerator.Generate(session, userName, ttl)
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

// HandleWorkerJoin handles POST /api/v1/worker/join requests.
func (h *WorkerHandler) HandleWorkerJoin(w http.ResponseWriter, r *http.Request) {
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

	validator := jointoken.NewValidator(h.jwtSecret)
	claims, err := validator.Validate(req.Token)
	if err != nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	userName := "tenant-" + claims.Session[:12]
	user, err := h.hsClient.GetUser(ctx, userName)
	if err != nil || user == nil {
		http.Error(w, "invalid session in token", http.StatusUnauthorized)
		return
	}

	key, err := h.tenantManager.CreateAuthKey(ctx, user.Name, 24*time.Hour, false)
	if err != nil {
		log.Printf("Failed to create auth key: %v", err)
		http.Error(w, "failed to create auth key", http.StatusInternalServerError)
		return
	}

	headscaleURL := h.headscalePublicURL
	if headscaleURL == "" {
		headscaleURL = h.headscaleURL
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"authkey":       key.Key,
		"headscale_url": headscaleURL,
		"user":          userName,
	})
}
