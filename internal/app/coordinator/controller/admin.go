package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// WonderNetResponse represents a wonder net in JSON responses.
type WonderNetResponse struct {
	ID          string `json:"id"`
	OwnerID     string `json:"owner_id"`
	DisplayName string `json:"display_name"`
	MeshType    string `json:"mesh_type"`
	CreatedAt   string `json:"created_at"`
}

// WonderNetListResponse represents the response for listing wonder nets.
type WonderNetListResponse struct {
	WonderNets []WonderNetResponse `json:"wonder_nets"`
	Count      int                 `json:"count"`
}

// AdminNodeResponse extends NodeResponse with wonder net info.
type AdminNodeResponse struct {
	NodeResponse
	WonderNetID string `json:"wonder_net_id"`
}

// AdminNodeListResponse represents the response for listing nodes across all wonder nets.
type AdminNodeListResponse struct {
	Nodes  []AdminNodeResponse `json:"nodes"`
	Count  int                 `json:"count"`
	Errors []string            `json:"errors,omitempty"`
}

// AdminController handles admin API endpoints.
type AdminController struct {
	wonderNetService *service.WonderNetService
	nodesService     *service.NodesService
}

// NewAdminController creates a new AdminController.
func NewAdminController(
	wonderNetService *service.WonderNetService,
	nodesService *service.NodesService,
) *AdminController {
	return &AdminController{
		wonderNetService: wonderNetService,
		nodesService:     nodesService,
	}
}

// HandleListWonderNets handles GET /admin/api/v1/wonder-nets requests.
func (c *AdminController) HandleListWonderNets(w http.ResponseWriter, r *http.Request) {
	wonderNets, err := c.wonderNetService.ListAllWonderNets(r.Context())
	if err != nil {
		slog.Error("list all wonder nets", "error", err)
		http.Error(w, "list wonder nets", http.StatusInternalServerError)
		return
	}

	result := make([]WonderNetResponse, len(wonderNets))
	for i, wn := range wonderNets {
		result[i] = WonderNetResponse{
			ID:          wn.ID,
			OwnerID:     wn.OwnerID,
			DisplayName: wn.DisplayName,
			MeshType:    wn.MeshType,
			CreatedAt:   wn.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(WonderNetListResponse{
		WonderNets: result,
		Count:      len(result),
	})
}

// HandleListWonderNetsByUser handles GET /admin/api/v1/users/{user_id}/wonder-nets requests.
func (c *AdminController) HandleListWonderNetsByUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("user_id")
	if userID == "" {
		http.Error(w, "user id required", http.StatusBadRequest)
		return
	}

	wonderNets, err := c.wonderNetService.ListWonderNetsByOwner(r.Context(), userID)
	if err != nil {
		slog.Error("list wonder nets by user", "error", err, "user_id", userID)
		http.Error(w, "list wonder nets", http.StatusInternalServerError)
		return
	}

	result := make([]WonderNetResponse, len(wonderNets))
	for i, wn := range wonderNets {
		result[i] = WonderNetResponse{
			ID:          wn.ID,
			OwnerID:     wn.OwnerID,
			DisplayName: wn.DisplayName,
			MeshType:    wn.MeshType,
			CreatedAt:   wn.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(WonderNetListResponse{
		WonderNets: result,
		Count:      len(result),
	})
}

// HandleListWonderNetNodes handles GET /admin/api/v1/wonder-nets/{id}/nodes requests.
func (c *AdminController) HandleListWonderNetNodes(w http.ResponseWriter, r *http.Request) {
	wonderNetID := r.PathValue("id")
	if wonderNetID == "" {
		http.Error(w, "wonder net id required", http.StatusBadRequest)
		return
	}

	wonderNet, err := c.wonderNetService.GetWonderNetByID(r.Context(), wonderNetID)
	if err != nil {
		slog.Error("get wonder net", "error", err, "id", wonderNetID)
		http.Error(w, "get wonder net", http.StatusInternalServerError)
		return
	}
	if wonderNet == nil {
		http.Error(w, "wonder net not found", http.StatusNotFound)
		return
	}

	nodes, err := c.nodesService.ListNodes(r.Context(), wonderNet)
	if err != nil {
		slog.Error("list nodes", "error", err, "wonder_net_id", wonderNetID)
		http.Error(w, "internal error", http.StatusInternalServerError)
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

// HandleListAllNodes handles GET /admin/api/v1/nodes requests.
func (c *AdminController) HandleListAllNodes(w http.ResponseWriter, r *http.Request) {
	wonderNets, err := c.wonderNetService.ListAllWonderNets(r.Context())
	if err != nil {
		slog.Error("list all wonder nets", "error", err)
		http.Error(w, "list wonder nets", http.StatusInternalServerError)
		return
	}

	var result []AdminNodeResponse
	var errors []string
	for _, wn := range wonderNets {
		nodes, err := c.nodesService.ListNodes(r.Context(), wn)
		if err != nil {
			slog.Warn("list nodes for wonder net", "error", err, "wonder_net_id", wn.ID)
			errors = append(errors, "wonder_net "+wn.ID+": "+err.Error())
			continue
		}
		for _, node := range nodes {
			resp := AdminNodeResponse{
				NodeResponse: NodeResponse{
					ID:      node.ID,
					Name:    node.Name,
					IPAddrs: node.IPAddrs,
					Online:  node.Online,
				},
				WonderNetID: wn.ID,
			}
			if node.LastSeen != nil {
				resp.LastSeen = node.LastSeen.Format("2006-01-02T15:04:05Z")
			}
			result = append(result, resp)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(AdminNodeListResponse{
		Nodes:  result,
		Count:  len(result),
		Errors: errors,
	})
}
