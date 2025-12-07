package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

// NodesHandler handles node-related requests.
type NodesHandler struct {
	tenantManager *headscale.TenantManager
	sessionStore  oidc.SessionStore
	userStore     oidc.UserStore
}

// NewNodesHandler creates a new NodesHandler.
func NewNodesHandler(
	tenantManager *headscale.TenantManager,
	sessionStore oidc.SessionStore,
	userStore oidc.UserStore,
) *NodesHandler {
	return &NodesHandler{
		tenantManager: tenantManager,
		sessionStore:  sessionStore,
		userStore:     userStore,
	}
}

// HandleListNodes handles GET /api/v1/nodes requests.
func (h *NodesHandler) HandleListNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sessionID := r.Header.Get("X-Session-Token")
	if sessionID == "" {
		http.Error(w, "session token required", http.StatusUnauthorized)
		return
	}

	session, err := h.sessionStore.Get(ctx, sessionID)
	if err != nil {
		log.Printf("Failed to get session: %v", err)
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}
	if session == nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	user, err := h.userStore.Get(ctx, session.UserID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}

	nodes, err := h.tenantManager.GetTenantNodes(ctx, user.HeadscaleUser)
	if err != nil {
		log.Printf("Failed to list nodes: %v", err)
		http.Error(w, "failed to list nodes", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
	})
}
