package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
)

// WorkerHandler handles worker-related requests.
type WorkerHandler struct {
	publicURL         string
	jwtSecret         string
	realmManager      *headscale.RealmManager
	tokenGenerator    *jointoken.Generator
	sessionRepository repository.SessionRepository
	realmRepository   repository.RealmRepository
}

// NewWorkerHandler creates a new WorkerHandler.
func NewWorkerHandler(
	publicURL string,
	jwtSecret string,
	realmManager *headscale.RealmManager,
	tokenGenerator *jointoken.Generator,
	sessionRepository repository.SessionRepository,
	realmRepository repository.RealmRepository,
) *WorkerHandler {
	return &WorkerHandler{
		publicURL:         publicURL,
		jwtSecret:         jwtSecret,
		realmManager:      realmManager,
		tokenGenerator:    tokenGenerator,
		sessionRepository: sessionRepository,
		realmRepository:   realmRepository,
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

	session, err := h.sessionRepository.Get(ctx, sessionID)
	if err != nil {
		slog.Error("get session", "error", err)
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}
	if session == nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	if err := h.sessionRepository.UpdateLastUsed(ctx, sessionID); err != nil {
		slog.Warn("update session last used", "error", err, "session_id", sessionID)
	}

	realms, err := h.realmRepository.ListByOwner(ctx, session.UserID)
	if err != nil {
		slog.Error("list user realms", "error", err)
		http.Error(w, "list realms", http.StatusInternalServerError)
		return
	}
	if len(realms) == 0 {
		http.Error(w, "no realm found", http.StatusBadRequest)
		return
	}
	realm := realms[0]

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

	token, err := h.tokenGenerator.Generate(realm.ID, realm.HeadscaleUser, ttl)
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

	realm, err := h.realmRepository.Get(ctx, claims.RealmID)
	if err != nil || realm == nil {
		http.Error(w, "invalid realm in token", http.StatusUnauthorized)
		return
	}

	key, err := h.realmManager.CreateAuthKeyByName(ctx, claims.HeadscaleUser, 24*time.Hour, false)
	if err != nil {
		slog.Error("create auth key", "error", err)
		http.Error(w, "create auth key", http.StatusInternalServerError)
		return
	}

	// headscale_url uses publicURL because the coordinator reverse-proxies
	// Tailscale control plane traffic to the embedded Headscale instance.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"authkey":       key.GetKey(),
		"headscale_url": h.publicURL,
		"user":          claims.HeadscaleUser,
	}); err != nil {
		slog.Error("encode worker join response", "error", err)
	}
}
