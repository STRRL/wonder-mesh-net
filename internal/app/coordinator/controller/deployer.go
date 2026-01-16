package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/meshbackend"
)

// DeployerController handles third-party PaaS deployer integration.
type DeployerController struct {
	meshBackend meshbackend.MeshBackend
}

// NewDeployerController creates a new DeployerController.
func NewDeployerController(meshBackend meshbackend.MeshBackend) *DeployerController {
	return &DeployerController{
		meshBackend: meshBackend,
	}
}

// HandleDeployerJoin handles POST /api/v1/deployer/join requests.
// This endpoint requires service account authentication via JWT.
// The wonder net is expected to be set in the request context by the JWT middleware.
func (c *DeployerController) HandleDeployerJoin(w http.ResponseWriter, r *http.Request) {
	wonderNet := WonderNetFromContext(r)
	if wonderNet == nil {
		http.Error(w, "authorization required", http.StatusUnauthorized)
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

	loginServer, ok := metadata["login_server"].(string)
	if !ok {
		slog.Error("invalid join credentials metadata: login_server missing or not a string")
		http.Error(w, "invalid join credentials", http.StatusInternalServerError)
		return
	}

	authkey, ok := metadata["authkey"].(string)
	if !ok {
		slog.Error("invalid join credentials metadata: authkey missing or not a string")
		http.Error(w, "invalid join credentials", http.StatusInternalServerError)
		return
	}

	headscaleUser, ok := metadata["headscale_user"].(string)
	if !ok {
		slog.Error("invalid join credentials metadata: headscale_user missing or not a string")
		http.Error(w, "invalid join credentials", http.StatusInternalServerError)
		return
	}

	resp := JoinCredentialsResponse{
		MeshType: meshType,
		TailscaleConnectionInfo: &TailscaleConnectionInfo{
			LoginServer:   loginServer,
			Authkey:       authkey,
			HeadscaleUser: headscaleUser,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
