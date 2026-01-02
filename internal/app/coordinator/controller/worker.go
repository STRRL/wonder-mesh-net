package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// JoinCredentialsResponse contains credentials for joining the mesh.
type JoinCredentialsResponse struct {
	MeshType                string                   `json:"mesh_type"`
	TailscaleConnectionInfo *TailscaleConnectionInfo `json:"tailscale_connection_info,omitempty"`
}

// TailscaleConnectionInfo contains the credentials for joining a Tailscale/Headscale mesh.
type TailscaleConnectionInfo struct {
	LoginServer   string `json:"login_server"`
	Authkey       string `json:"authkey"`
	HeadscaleUser string `json:"headscale_user"`
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

	if creds.MeshType != "tailscale" {
		slog.Error("unsupported mesh type", "mesh_type", creds.MeshType)
		http.Error(w, "unsupported mesh type", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	resp := JoinCredentialsResponse{
		MeshType: creds.MeshType,
		TailscaleConnectionInfo: &TailscaleConnectionInfo{
			LoginServer:   creds.Metadata["login_server"].(string),
			Authkey:       creds.Metadata["authkey"].(string),
			HeadscaleUser: creds.Metadata["headscale_user"].(string),
		},
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("encode worker join response", "error", err)
	}
}
