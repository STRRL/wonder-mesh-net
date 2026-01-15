package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
	"github.com/strrl/wonder-mesh-net/pkg/meshbackend"
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
	workerService    *service.WorkerService
	apiKeyService    *service.APIKeyService
	meshBackend      meshbackend.MeshBackend
}

// NewAdminController creates a new AdminController.
func NewAdminController(
	wonderNetService *service.WonderNetService,
	nodesService *service.NodesService,
	workerService *service.WorkerService,
	apiKeyService *service.APIKeyService,
	meshBackend meshbackend.MeshBackend,
) *AdminController {
	return &AdminController{
		wonderNetService: wonderNetService,
		nodesService:     nodesService,
		workerService:    workerService,
		apiKeyService:    apiKeyService,
		meshBackend:      meshBackend,
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

// AdminCreateWonderNetRequest represents the request to create a wonder net.
type AdminCreateWonderNetRequest struct {
	OwnerID     string `json:"owner_id"`
	DisplayName string `json:"display_name"`
}

// HandleAdminCreateWonderNet handles POST /admin/api/v1/wonder-nets requests.
func (c *AdminController) HandleAdminCreateWonderNet(w http.ResponseWriter, r *http.Request) {
	var req AdminCreateWonderNetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.OwnerID == "" {
		http.Error(w, "owner_id is required", http.StatusBadRequest)
		return
	}

	displayName := req.DisplayName
	if displayName == "" {
		displayName = "Admin Created Wonder Net"
	}

	wonderNet, err := c.wonderNetService.ProvisionWonderNet(r.Context(), req.OwnerID, displayName)
	if err != nil {
		slog.Error("provision wonder net", "error", err)
		http.Error(w, "provision wonder net", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(WonderNetResponse{
		ID:          wonderNet.ID,
		OwnerID:     wonderNet.OwnerID,
		DisplayName: wonderNet.DisplayName,
		MeshType:    wonderNet.MeshType,
		CreatedAt:   wonderNet.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// HandleAdminCreateJoinToken handles POST /admin/api/v1/wonder-nets/{id}/join-token requests.
func (c *AdminController) HandleAdminCreateJoinToken(w http.ResponseWriter, r *http.Request) {
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

	token, err := c.workerService.GenerateJoinToken(r.Context(), wonderNet, 8*time.Hour)
	if err != nil {
		slog.Error("generate join token", "error", err)
		http.Error(w, "generate join token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(JoinTokenResponse{
		Token:     token,
		ExpiresIn: 28800,
	})
}

// HandleAdminCreateAPIKey handles POST /admin/api/v1/wonder-nets/{id}/api-keys requests.
func (c *AdminController) HandleAdminCreateAPIKey(w http.ResponseWriter, r *http.Request) {
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

// HandleAdminDeployerJoin handles POST /admin/api/v1/wonder-nets/{id}/deployer/join requests.
func (c *AdminController) HandleAdminDeployerJoin(w http.ResponseWriter, r *http.Request) {
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

	metadata, err := c.meshBackend.CreateJoinCredentials(r.Context(), wonderNet.HeadscaleUser, meshbackend.JoinOptions{
		TTL:       24 * time.Hour,
		Reusable:  false,
		Ephemeral: false,
	})
	if err != nil {
		slog.Error("create join credentials", "error", err)
		http.Error(w, "create join credentials", http.StatusInternalServerError)
		return
	}

	meshType := string(c.meshBackend.MeshType())
	if meshType != "tailscale" {
		slog.Error("unsupported mesh type", "mesh_type", meshType)
		http.Error(w, "unsupported mesh type", http.StatusInternalServerError)
		return
	}

	resp := JoinCredentialsResponse{
		MeshType: meshType,
		TailscaleConnectionInfo: &TailscaleConnectionInfo{
			LoginServer:   metadata["login_server"].(string),
			Authkey:       metadata["authkey"].(string),
			HeadscaleUser: metadata["headscale_user"].(string),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
