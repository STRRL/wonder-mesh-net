package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/store"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
)

// WorkerHandler handles worker-related requests.
type WorkerHandler struct {
	publicURL      string
	jwtSecret      string
	realmManager   *headscale.RealmManager
	tokenGenerator *jointoken.Generator
	sessionStore   store.SessionStore
	userStore      store.UserStore
}

// NewWorkerHandler creates a new WorkerHandler.
func NewWorkerHandler(
	publicURL string,
	jwtSecret string,
	realmManager *headscale.RealmManager,
	tokenGenerator *jointoken.Generator,
	sessionStore store.SessionStore,
	userStore store.UserStore,
) *WorkerHandler {
	return &WorkerHandler{
		publicURL:      publicURL,
		jwtSecret:      jwtSecret,
		realmManager:   realmManager,
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
		slog.Error("get session", "error", err)
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}
	if session == nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	if err := h.sessionStore.UpdateLastUsed(ctx, sessionID); err != nil {
		slog.Warn("update session last used", "error", err, "session_id", sessionID)
	}

	user, err := h.userStore.Get(ctx, session.UserID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}

	var req struct {
		TTL string `json:"ttl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.TTL = "15m"
	}

	ttl := 15 * time.Minute
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
		slog.Error("generate join token", "error", err)
		http.Error(w, "generate join token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"token":   token,
		"command": fmt.Sprintf("wonder worker join %s", token),
	}); err != nil {
		slog.Error("encode join token response", "error", err)
	}
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

	key, err := h.realmManager.CreateAuthKeyByName(ctx, claims.HeadscaleUser, 24*time.Hour, false)
	if err != nil {
		slog.Error("create auth key", "error", err)
		http.Error(w, "create auth key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"authkey":       key.GetKey(),
		"headscale_url": h.publicURL,
		"user":          claims.HeadscaleUser,
	}); err != nil {
		slog.Error("encode worker join response", "error", err)
	}
}
