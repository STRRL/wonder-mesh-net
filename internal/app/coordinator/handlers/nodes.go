package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/store"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
)

// NodesHandler handles node-related requests.
type NodesHandler struct {
	realmManager *headscale.RealmManager
	apiKeyStore  store.APIKeyStore
	auth         *AuthHelper
}

// NewNodesHandler creates a new NodesHandler.
func NewNodesHandler(
	realmManager *headscale.RealmManager,
	apiKeyStore store.APIKeyStore,
	auth *AuthHelper,
) *NodesHandler {
	return &NodesHandler{
		realmManager: realmManager,
		apiKeyStore:  apiKeyStore,
		auth:         auth,
	}
}

// HandleListNodes handles GET /api/v1/nodes requests.
// Supports two authentication methods:
// 1. X-Session-Token header (realm session)
// 2. Authorization: Bearer <api_key> header (third-party integration)
func (h *NodesHandler) HandleListNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	realm, err := h.authenticate(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	nodes, err := h.realmManager.GetRealmNodes(ctx, realm.HeadscaleUser)
	if err != nil {
		slog.Error("list nodes", "error", err)
		http.Error(w, "list nodes", http.StatusInternalServerError)
		return
	}

	result := make([]NodeResponse, len(nodes))
	for i, node := range nodes {
		result[i] = NodeResponse{
			ID:      node.GetId(),
			Name:    node.GetName(),
			IPAddrs: node.GetIpAddresses(),
			Online:  node.GetOnline(),
		}
		if node.GetLastSeen() != nil {
			result[i].LastSeen = node.GetLastSeen().AsTime().Format("2006-01-02T15:04:05Z")
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(NodeListResponse{
		Nodes: result,
		Count: len(result),
	})
}

func (h *NodesHandler) authenticate(r *http.Request) (*store.Realm, error) {
	ctx := r.Context()

	if key := GetBearerToken(r); key != "" {
		apiKey, err := h.apiKeyStore.GetByKey(ctx, key)
		if err != nil {
			return nil, err
		}
		if apiKey == nil {
			return nil, ErrInvalidAPIKey
		}

		if err := h.apiKeyStore.UpdateLastUsed(ctx, apiKey.ID); err != nil {
			slog.Warn("update API key last used", "error", err)
		}

		return h.auth.GetRealmByID(ctx, apiKey.RealmID)
	}

	return h.auth.AuthenticateSession(r)
}
