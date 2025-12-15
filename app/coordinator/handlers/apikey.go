package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/strrl/wonder-mesh-net/pkg/apikey"
)

// APIKeyHandler handles API key management requests.
type APIKeyHandler struct {
	apiKeyStore apikey.Store
	auth        *AuthHelper
}

// NewAPIKeyHandler creates a new APIKeyHandler.
func NewAPIKeyHandler(apiKeyStore apikey.Store, auth *AuthHelper) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyStore: apiKeyStore,
		auth:        auth,
	}
}

// HandleCreateAPIKey handles POST /api/v1/api-keys requests.
func (h *APIKeyHandler) HandleCreateAPIKey(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()

	user, err := h.auth.AuthenticateSession(r)
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
	_ = json.NewEncoder(w).Encode(APIKeyResponse{
		ID:        apiKeyWithSecret.ID,
		Key:       apiKeyWithSecret.Key,
		Name:      apiKeyWithSecret.Name,
		Scopes:    apiKeyWithSecret.Scopes,
		CreatedAt: apiKeyWithSecret.CreatedAt,
		ExpiresAt: apiKeyWithSecret.ExpiresAt,
	})
}

// HandleListAPIKeys handles GET /api/v1/api-keys requests.
func (h *APIKeyHandler) HandleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user, err := h.auth.AuthenticateSession(r)
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

	result := make([]APIKeyResponse, len(keys))
	for i, key := range keys {
		result[i] = APIKeyResponse{
			ID:         key.ID,
			Key:        key.Key,
			Name:       key.Name,
			Scopes:     key.Scopes,
			CreatedAt:  key.CreatedAt,
			ExpiresAt:  key.ExpiresAt,
			LastUsedAt: key.LastUsedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(APIKeyListResponse{APIKeys: result})
}

// HandleDeleteAPIKey handles DELETE /api/v1/api-keys/{id} requests.
func (h *APIKeyHandler) HandleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user, err := h.auth.AuthenticateSession(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	keyID := chi.URLParam(r, "id")
	if keyID == "" {
		http.Error(w, "key ID required", http.StatusBadRequest)
		return
	}

	if err := h.apiKeyStore.Delete(ctx, keyID, user.ID); err != nil {
		if errors.Is(err, apikey.ErrNotFound) {
			http.Error(w, "api key not found", http.StatusNotFound)
			return
		}
		slog.Error("failed to delete API key", "error", err)
		http.Error(w, "failed to delete API key", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
