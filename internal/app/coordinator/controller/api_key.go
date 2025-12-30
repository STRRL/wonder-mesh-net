package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// APIKeyController handles API key management endpoints.
type APIKeyController struct {
	apiKeyService *service.APIKeyService
}

// NewAPIKeyController creates a new APIKeyController.
func NewAPIKeyController(apiKeyService *service.APIKeyService) *APIKeyController {
	return &APIKeyController{
		apiKeyService: apiKeyService,
	}
}

// CreateAPIKeyRequest is the request body for creating an API key.
type CreateAPIKeyRequest struct {
	Name      string `json:"name"`
	ExpiresIn string `json:"expires_in,omitempty"`
}

// CreateAPIKeyResponse is the response body for creating an API key.
type CreateAPIKeyResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"`
	KeyPrefix string     `json:"key_prefix"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// HandleCreate handles POST /api/v1/api-keys requests.
func (c *APIKeyController) HandleCreate(w http.ResponseWriter, r *http.Request) {
	wonderNet := WonderNetFromContext(r)
	if wonderNet == nil {
		http.Error(w, "authorization required", http.StatusUnauthorized)
		return
	}

	var req CreateAPIKeyRequest
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
		if duration <= 0 {
			http.Error(w, "expires_in must be a positive duration", http.StatusBadRequest)
			return
		}
		t := time.Now().Add(duration)
		expiresAt = &t
	}

	details, err := c.apiKeyService.CreateAPIKey(r.Context(), wonderNet.ID, req.Name, expiresAt)
	if err != nil {
		slog.Error("create api key", "error", err)
		http.Error(w, "create api key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(CreateAPIKeyResponse{
		ID:        details.ID,
		Name:      details.Name,
		Key:       details.Key,
		KeyPrefix: details.KeyPrefix,
		ExpiresAt: details.ExpiresAt,
	})
}

// APIKeyInfoResponse is the response for listing API keys.
type APIKeyInfoResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// HandleList handles GET /api/v1/api-keys requests.
func (c *APIKeyController) HandleList(w http.ResponseWriter, r *http.Request) {
	wonderNet := WonderNetFromContext(r)
	if wonderNet == nil {
		http.Error(w, "authorization required", http.StatusUnauthorized)
		return
	}

	keys, err := c.apiKeyService.ListAPIKeys(r.Context(), wonderNet.ID)
	if err != nil {
		slog.Error("list api keys", "error", err)
		http.Error(w, "list api keys", http.StatusInternalServerError)
		return
	}

	response := make([]APIKeyInfoResponse, len(keys))
	for i, key := range keys {
		response[i] = APIKeyInfoResponse{
			ID:         key.ID,
			Name:       key.Name,
			KeyPrefix:  key.KeyPrefix,
			CreatedAt:  key.CreatedAt,
			LastUsedAt: key.LastUsedAt,
			ExpiresAt:  key.ExpiresAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// HandleDelete handles DELETE /api/v1/api-keys/{id} requests.
func (c *APIKeyController) HandleDelete(w http.ResponseWriter, r *http.Request) {
	wonderNet := WonderNetFromContext(r)
	if wonderNet == nil {
		http.Error(w, "authorization required", http.StatusUnauthorized)
		return
	}

	keyID := r.PathValue("id")
	if keyID == "" {
		http.Error(w, "missing key id", http.StatusBadRequest)
		return
	}

	err := c.apiKeyService.DeleteAPIKey(r.Context(), wonderNet.ID, keyID)
	if err != nil {
		if err == service.ErrAPIKeyNotFound {
			http.Error(w, "api key not found", http.StatusNotFound)
			return
		}
		slog.Error("delete api key", "error", err)
		http.Error(w, "delete api key", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
