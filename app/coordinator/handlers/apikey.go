package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/apikey"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

// APIKeyHandler handles API key management requests.
type APIKeyHandler struct {
	apiKeyStore  apikey.Store
	sessionStore oidc.SessionStore
	userStore    oidc.UserStore
}

// NewAPIKeyHandler creates a new APIKeyHandler.
func NewAPIKeyHandler(
	apiKeyStore apikey.Store,
	sessionStore oidc.SessionStore,
	userStore oidc.UserStore,
) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyStore:  apiKeyStore,
		sessionStore: sessionStore,
		userStore:    userStore,
	}
}

// HandleAPIKeys handles GET/POST /api/v1/api-keys requests.
func (h *APIKeyHandler) HandleAPIKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.HandleListAPIKeys(w, r)
	case http.MethodPost:
		h.handleCreateAPIKey(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCreateAPIKey handles POST /api/v1/api-keys requests.
func (h *APIKeyHandler) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()

	user, err := h.authenticateSession(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name      string `json:"name"`
		Scopes    string `json:"scopes"`
		ExpiresIn string `json:"expires_in"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	scopes := req.Scopes
	if scopes == "" {
		scopes = "nodes:read"
	}

	var expiresAt *time.Time
	if req.ExpiresIn != "" {
		duration, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			http.Error(w, "invalid expires_in format", http.StatusBadRequest)
			return
		}
		t := time.Now().Add(duration)
		expiresAt = &t
	}

	apiKeyWithSecret, err := h.apiKeyStore.Create(ctx, user.ID, req.Name, scopes, expiresAt)
	if err != nil {
		slog.Error("failed to create API key", "error", err)
		http.Error(w, "failed to create API key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":         apiKeyWithSecret.ID,
		"key":        apiKeyWithSecret.Key,
		"name":       apiKeyWithSecret.Name,
		"scopes":     apiKeyWithSecret.Scopes,
		"created_at": apiKeyWithSecret.CreatedAt,
		"expires_at": apiKeyWithSecret.ExpiresAt,
	})
}

// HandleListAPIKeys handles GET /api/v1/api-keys requests.
func (h *APIKeyHandler) HandleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user, err := h.authenticateSession(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	keys, err := h.apiKeyStore.List(ctx, user.ID)
	if err != nil {
		slog.Error("failed to list API keys", "error", err)
		http.Error(w, "failed to list API keys", http.StatusInternalServerError)
		return
	}

	result := make([]map[string]any, len(keys))
	for i, key := range keys {
		result[i] = map[string]any{
			"id":           key.ID,
			"key":          key.Key,
			"name":         key.Name,
			"scopes":       key.Scopes,
			"created_at":   key.CreatedAt,
			"expires_at":   key.ExpiresAt,
			"last_used_at": key.LastUsedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"api_keys": result,
	})
}

// HandleDeleteAPIKey handles DELETE /api/v1/api-keys/{id} requests.
func (h *APIKeyHandler) HandleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	user, err := h.authenticateSession(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	keyID := strings.TrimPrefix(r.URL.Path, "/api/v1/api-keys/")
	if keyID == "" || keyID == r.URL.Path {
		http.Error(w, "key ID required", http.StatusBadRequest)
		return
	}

	if err := h.apiKeyStore.Delete(ctx, keyID, user.ID); err != nil {
		slog.Error("failed to delete API key", "error", err)
		http.Error(w, "failed to delete API key", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *APIKeyHandler) authenticateSession(r *http.Request) (*oidc.User, error) {
	ctx := r.Context()

	sessionID := r.Header.Get("X-Session-Token")
	if sessionID == "" {
		return nil, http.ErrNoCookie
	}

	session, err := h.sessionStore.Get(ctx, sessionID)
	if err != nil || session == nil {
		return nil, http.ErrNoCookie
	}

	user, err := h.userStore.Get(ctx, session.UserID)
	if err != nil || user == nil {
		return nil, http.ErrNoCookie
	}

	return user, nil
}
