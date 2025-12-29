package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// DeployerController handles third-party PaaS deployer integration.
type DeployerController struct {
	wonderNetService *service.WonderNetService
}

// NewDeployerController creates a new DeployerController.
func NewDeployerController(wonderNetService *service.WonderNetService) *DeployerController {
	return &DeployerController{
		wonderNetService: wonderNetService,
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

	authKey, err := c.wonderNetService.CreateAuthKey(r.Context(), wonderNet, 24*time.Hour, false)
	if err != nil {
		slog.Error("create auth key", "error", err)
		http.Error(w, "create auth key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(JoinCredentialsResponse{
		AuthKey:      authKey,
		HeadscaleURL: c.wonderNetService.GetPublicURL(),
		User:         wonderNet.HeadscaleUser,
	})
}
