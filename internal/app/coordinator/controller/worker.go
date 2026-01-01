package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// JoinCredentialsResponse contains credentials for joining the mesh.
// The connection info field is named dynamically based on mesh_type:
// - tailscale_connection_info for Tailscale
// - netbird_connection_info for Netbird
// - zerotier_connection_info for ZeroTier
type JoinCredentialsResponse struct {
	MeshType string `json:"mesh_type"`

	TailscaleConnectionInfo map[string]any `json:"tailscale_connection_info,omitempty"`
	NetbirdConnectionInfo   map[string]any `json:"netbird_connection_info,omitempty"`
	ZerotierConnectionInfo  map[string]any `json:"zerotier_connection_info,omitempty"`
}

// WorkerController handles worker node registration.
type WorkerController struct {
	workerService *service.WorkerService
}

// NewWorkerController creates a new WorkerController.
func NewWorkerController(workerService *service.WorkerService) *WorkerController {
	return &WorkerController{
		workerService: workerService,
	}
}

// HandleWorkerJoin handles POST /api/v1/worker/join requests.
// This endpoint doesn't require auth - it validates the join token itself.
func (c *WorkerController) HandleWorkerJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	creds, err := c.workerService.ExchangeJoinToken(r.Context(), req.Token)
	if err != nil {
		if err == service.ErrInvalidToken {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		} else {
			slog.Error("exchange join token", "error", err)
			http.Error(w, "exchange join token", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	resp := JoinCredentialsResponse{
		MeshType: creds.MeshType,
	}

	switch creds.MeshType {
	case "tailscale":
		resp.TailscaleConnectionInfo = creds.Metadata
	case "netbird":
		resp.NetbirdConnectionInfo = creds.Metadata
	case "zerotier":
		resp.ZerotierConnectionInfo = creds.Metadata
	default:
		slog.Error("unsupported mesh type", "mesh_type", creds.MeshType)
		http.Error(w, "unsupported mesh type", http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("encode worker join response", "error", err)
	}
}
