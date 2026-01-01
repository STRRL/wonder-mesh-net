package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// JoinCredentialsResponse contains credentials for joining the mesh.
type JoinCredentialsResponse struct {
	// MeshType identifies the mesh network type (e.g., "tailscale", "netbird")
	MeshType string `json:"mesh_type"`

	// Metadata contains mesh-specific credentials
	// For Tailscale: login_server, authkey, headscale_user
	Metadata map[string]any `json:"metadata"`

	// Legacy fields for backward compatibility
	AuthKey       string `json:"authkey,omitempty"`
	HeadscaleURL  string `json:"headscale_url,omitempty"`
	HeadscaleUser string `json:"headscale_user,omitempty"`
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
		Metadata: creds.Metadata,
	}

	// Populate legacy fields for backward compatibility with Tailscale
	if creds.MeshType == "tailscale" {
		if loginServer, ok := creds.Metadata["login_server"].(string); ok {
			resp.HeadscaleURL = loginServer
		}
		if authkey, ok := creds.Metadata["authkey"].(string); ok {
			resp.AuthKey = authkey
		}
		if hsUser, ok := creds.Metadata["headscale_user"].(string); ok {
			resp.HeadscaleUser = hsUser
		}
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("encode worker join response", "error", err)
	}
}
