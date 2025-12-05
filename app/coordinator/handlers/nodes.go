package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/strrl/wonder-mesh-net/pkg/headscale"
)

// NodesHandler handles node-related requests.
type NodesHandler struct {
	hsClient      *headscale.Client
	tenantManager *headscale.TenantManager
}

// NewNodesHandler creates a new NodesHandler.
func NewNodesHandler(hsClient *headscale.Client, tenantManager *headscale.TenantManager) *NodesHandler {
	return &NodesHandler{
		hsClient:      hsClient,
		tenantManager: tenantManager,
	}
}

// HandleListNodes handles GET /api/v1/nodes requests.
func (h *NodesHandler) HandleListNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := r.Header.Get("X-Session-Token")
	if session == "" {
		http.Error(w, "session token required", http.StatusUnauthorized)
		return
	}

	userName := "tenant-" + session[:12]
	user, err := h.hsClient.GetUser(ctx, userName)
	if err != nil || user == nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	nodes, err := h.tenantManager.GetTenantNodes(ctx, user.ID)
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
