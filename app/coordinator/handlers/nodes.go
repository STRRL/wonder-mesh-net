package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/strrl/wonder-mesh-net/pkg/apikey"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

// NodesHandler handles node-related requests.
type NodesHandler struct {
	realmManager *headscale.RealmManager
	sessionStore oidc.SessionStore
	userStore    oidc.UserStore
	apiKeyStore  apikey.Store
}

// NewNodesHandler creates a new NodesHandler.
func NewNodesHandler(
	realmManager *headscale.RealmManager,
	sessionStore oidc.SessionStore,
	userStore oidc.UserStore,
	apiKeyStore apikey.Store,
) *NodesHandler {
	return &NodesHandler{
		realmManager: realmManager,
		sessionStore: sessionStore,
		userStore:    userStore,
		apiKeyStore:  apiKeyStore,
	}
}

// HandleListNodes handles GET /api/v1/nodes requests.
// Supports two authentication methods:
// 1. X-Session-Token header (user session)
// 2. Authorization: Bearer <api_key> header (third-party integration)
func (h *NodesHandler) HandleListNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user, err := h.authenticate(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	nodes, err := h.realmManager.GetRealmNodes(ctx, user.HeadscaleUser)
	if err != nil {
		slog.Error("failed to list nodes", "error", err)
		http.Error(w, "failed to list nodes", http.StatusInternalServerError)
		return
	}

	type nodeInfo struct {
		ID       uint64   `json:"id"`
		Name     string   `json:"name"`
		IPAddrs  []string `json:"ip_addresses"`
		Online   bool     `json:"online"`
		LastSeen string   `json:"last_seen,omitempty"`
	}

	result := make([]nodeInfo, len(nodes))
	for i, node := range nodes {
		result[i] = nodeInfo{
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
	_ = json.NewEncoder(w).Encode(map[string]any{
		"nodes": result,
		"count": len(result),
	})
}

func (h *NodesHandler) authenticate(r *http.Request) (*oidc.User, error) {
	ctx := r.Context()

	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		if strings.HasPrefix(authHeader, "Bearer ") {
			key := strings.TrimPrefix(authHeader, "Bearer ")
			apiKey, err := h.apiKeyStore.GetByKey(ctx, key)
			if err != nil {
				return nil, err
			}
			if apiKey == nil {
				return nil, http.ErrNoCookie
			}

			if !strings.Contains(apiKey.Scopes, "nodes:read") {
				return nil, http.ErrNoCookie
			}

			if err := h.apiKeyStore.UpdateLastUsed(ctx, apiKey.ID); err != nil {
				slog.Warn("failed to update API key last used", "error", err)
			}

			user, err := h.userStore.Get(ctx, apiKey.UserID)
			if err != nil || user == nil {
				return nil, http.ErrNoCookie
			}
			return user, nil
		}
	}

	sessionID := r.Header.Get("X-Session-Token")
	if sessionID != "" {
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

	return nil, http.ErrNoCookie
}
