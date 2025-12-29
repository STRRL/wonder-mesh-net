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
}

// NewNodesController creates a new NodesController.
func NewNodesController(nodesService *service.NodesService) *NodesController {
	return &NodesController{
		nodesService: nodesService,
	}
}

// HandleListNodes handles GET /api/v1/nodes requests.
// This endpoint requires JWT authentication - the wonder net is expected to be
// set in the request context by the JWT middleware.
func (c *NodesController) HandleListNodes(w http.ResponseWriter, r *http.Request) {
	wonderNet := WonderNetFromContext(r)
	if wonderNet == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	nodes, err := c.nodesService.ListNodes(r.Context(), wonderNet)
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
