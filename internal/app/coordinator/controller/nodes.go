package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// NodesController handles node listing.
type NodesController struct {
	nodesService *service.NodesService
	authService  *service.AuthService
}

// NewNodesController creates a new NodesController.
func NewNodesController(
	nodesService *service.NodesService,
	authService *service.AuthService,
) *NodesController {
	return &NodesController{
		nodesService: nodesService,
		authService:  authService,
	}
}

// HandleListNodes handles GET /api/v1/nodes requests.
// Accepts both session tokens and API keys (read-only, safe for third-party integrations).
func (c *NodesController) HandleListNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	realm, err := c.authService.GetRealmFromRequest(ctx, r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	nodes, err := c.nodesService.ListNodes(ctx, realm)
	if err != nil {
		slog.Error("list nodes", "error", err)
		http.Error(w, "list nodes", http.StatusInternalServerError)
		return
	}

	result := make([]NodeResponse, len(nodes))
	for i, node := range nodes {
		result[i] = NodeResponse{
			ID:      node.ID,
			Name:    node.Name,
			IPAddrs: node.IPAddrs,
			Online:  node.Online,
		}
		if node.LastSeen != nil {
			result[i].LastSeen = node.LastSeen.Format("2006-01-02T15:04:05Z")
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(NodeListResponse{
		Nodes: result,
		Count: len(result),
	})
}
