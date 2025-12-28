package controller

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// APIKeyController manages API key CRUD operations.
type APIKeyController struct {
	apiKeyRepository *repository.APIKeyRepository
	authService      *service.AuthService
}

// NewAPIKeyController creates a new APIKeyController.
func NewAPIKeyController(
	apiKeyRepository *repository.APIKeyRepository,
	authService *service.AuthService,
) *APIKeyController {
	return &APIKeyController{
		apiKeyRepository: apiKeyRepository,
		authService:      authService,
	}
}

// HandleCreateAPIKey handles POST /api/v1/api-keys requests.
func (c *APIKeyController) HandleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	realm, err := c.authService.GetRealmFromRequest(ctx, r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name      string `json:"name"`
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

	apiKeyWithSecret, err := c.apiKeyRepository.Create(ctx, realm.ID, req.Name, expiresAt)
	if err != nil {
		slog.Error("create API key", "error", err)
		http.Error(w, "create API key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(APIKeyResponse{
		ID:        apiKeyWithSecret.ID,
		Key:       apiKeyWithSecret.Key,
		Name:      apiKeyWithSecret.Name,
		CreatedAt: apiKeyWithSecret.CreatedAt,
		ExpiresAt: apiKeyWithSecret.ExpiresAt,
	})
}

// HandleListAPIKeys handles GET /api/v1/api-keys requests.
func (c *APIKeyController) HandleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	realm, err := c.authService.GetRealmFromRequest(ctx, r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	keys, err := c.apiKeyRepository.List(ctx, realm.ID)
	if err != nil {
		slog.Error("list API keys", "error", err)
		http.Error(w, "list API keys", http.StatusInternalServerError)
		return
	}

	result := make([]APIKeyResponse, len(keys))
	for i, key := range keys {
		result[i] = APIKeyResponse{
			ID:         key.ID,
			Key:        key.Key,
			Name:       key.Name,
			CreatedAt:  key.CreatedAt,
			ExpiresAt:  key.ExpiresAt,
			LastUsedAt: key.LastUsedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(APIKeyListResponse{APIKeys: result})
}

// HandleDeleteAPIKey handles DELETE /api/v1/api-keys/{id} requests.
func (c *APIKeyController) HandleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	realm, err := c.authService.GetRealmFromRequest(ctx, r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	keyID := chi.URLParam(r, "id")
	if keyID == "" {
		http.Error(w, "key ID required", http.StatusBadRequest)
		return
	}

	if err := c.apiKeyRepository.Delete(ctx, keyID, realm.ID); err != nil {
		if errors.Is(err, repository.ErrAPIKeyNotFound) {
			http.Error(w, "api key not found", http.StatusNotFound)
			return
		}
		slog.Error("delete API key", "error", err)
		http.Error(w, "delete API key", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
