package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/store"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
)

// DeployerHandler handles deployer integration requests.
type DeployerHandler struct {
	publicURL    string
	realmManager *headscale.RealmManager
	apiKeyStore  store.APIKeyStore
	auth         *AuthHelper
}

// NewDeployerHandler creates a new DeployerHandler.
func NewDeployerHandler(
	publicURL string,
	realmManager *headscale.RealmManager,
	apiKeyStore store.APIKeyStore,
	auth *AuthHelper,
) *DeployerHandler {
	return &DeployerHandler{
		publicURL:    publicURL,
		realmManager: realmManager,
		apiKeyStore:  apiKeyStore,
		auth:         auth,
	}
}

// HandleDeployerJoin handles POST /api/v1/deployer/join requests.
// Allows third-party deployers to join user's mesh using API key.
func (h *DeployerHandler) HandleDeployerJoin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	key := GetBearerToken(r)
	if key == "" {
		http.Error(w, "authorization required", http.StatusUnauthorized)
		return
	}

	apiKey, err := h.apiKeyStore.GetByKey(ctx, key)
	if err != nil {
		slog.Error("failed to get API key", "error", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if apiKey == nil {
		http.Error(w, "invalid API key", http.StatusUnauthorized)
		return
	}

	if !store.HasScope(apiKey.Scopes, "deployer:connect") {
		http.Error(w, "insufficient scope: deployer:connect required", http.StatusForbidden)
		return
	}

	if err := h.apiKeyStore.UpdateLastUsed(ctx, apiKey.ID); err != nil {
		slog.Warn("failed to update API key last used", "error", err)
	}

	user, err := h.auth.GetUserByID(ctx, apiKey.UserID)
	if err != nil {
		slog.Error("failed to get user", "error", err)
		http.Error(w, "user not found", http.StatusInternalServerError)
		return
	}

	authKey, err := h.realmManager.CreateAuthKeyByName(ctx, user.HeadscaleUser, 24*time.Hour, false)
	if err != nil {
		slog.Error("failed to create auth key", "error", err)
		http.Error(w, "failed to create auth key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(DeployerJoinResponse{
		AuthKey:      authKey.GetKey(),
		HeadscaleURL: h.publicURL,
		User:         user.HeadscaleUser,
	})
}
