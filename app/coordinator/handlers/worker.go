package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

// WorkerHandler handles worker-related requests.
type WorkerHandler struct {
	publicURL      string
	jwtSecret      string
	tenantManager  *headscale.TenantManager
	tokenGenerator *jointoken.Generator
	sessionStore   oidc.SessionStore
	userStore      oidc.UserStore
}

// NewWorkerHandler creates a new WorkerHandler.
func NewWorkerHandler(
	publicURL string,
	jwtSecret string,
	tenantManager *headscale.TenantManager,
	tokenGenerator *jointoken.Generator,
	sessionStore oidc.SessionStore,
	userStore oidc.UserStore,
) *WorkerHandler {
	return &WorkerHandler{
		publicURL:      publicURL,
		jwtSecret:      jwtSecret,
		tenantManager:  tenantManager,
		tokenGenerator: tokenGenerator,
		sessionStore:   sessionStore,
		userStore:      userStore,
	}
}

// HandleCreateJoinToken handles POST /api/v1/join-token requests.
func (h *WorkerHandler) HandleCreateJoinToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	sessionID := r.Header.Get("X-Session-Token")
	if sessionID == "" {
		http.Error(w, "session token required", http.StatusUnauthorized)
		return
	}

	session, err := h.sessionStore.Get(ctx, sessionID)
	if err != nil {
		log.Printf("Failed to get session: %v", err)
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}
	if session == nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	_ = h.sessionStore.UpdateLastUsed(ctx, sessionID)

	user, err := h.userStore.Get(ctx, session.UserID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusUnauthorized)
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

	token, err := h.tokenGenerator.Generate(user.ID, user.HeadscaleUser, ttl)
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

	user, err := h.userStore.Get(ctx, claims.UserID)
	if err != nil || user == nil {
		http.Error(w, "invalid user in token", http.StatusUnauthorized)
		return
	}

	key, err := h.tenantManager.CreateAuthKeyByName(ctx, claims.HeadscaleUser, 24*time.Hour, false)
	if err != nil {
		log.Printf("Failed to create auth key: %v", err)
		http.Error(w, "failed to create auth key", http.StatusInternalServerError)
		return
	}

	headscaleURL := strings.Replace(h.publicURL, ":9080", ":8080", 1)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"authkey":       key.GetKey(),
		"headscale_url": headscaleURL,
		"user":          claims.HeadscaleUser,
	})
}
