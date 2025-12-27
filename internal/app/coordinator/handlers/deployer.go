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
		slog.Error("get API key", "error", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if apiKey == nil {
		http.Error(w, "invalid API key", http.StatusUnauthorized)
		return
	}

	if err := h.apiKeyStore.UpdateLastUsed(ctx, apiKey.ID); err != nil {
		slog.Warn("update API key last used", "error", err)
	}

	realm, err := h.auth.GetRealmByID(ctx, apiKey.RealmID)
	if err != nil {
		slog.Error("get realm", "error", err)
		http.Error(w, "realm not found", http.StatusInternalServerError)
		return
	}

	authKey, err := h.realmManager.CreateAuthKeyByName(ctx, realm.HeadscaleUser, 24*time.Hour, false)
	if err != nil {
		slog.Error("create auth key", "error", err)
		http.Error(w, "create auth key", http.StatusInternalServerError)
		return
	}

	// HeadscaleURL uses publicURL because the coordinator reverse-proxies
	// Tailscale control plane traffic to the embedded Headscale instance.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(DeployerJoinResponse{
		AuthKey:      authKey.GetKey(),
		HeadscaleURL: h.publicURL,
		User:         realm.HeadscaleUser,
	})
}
