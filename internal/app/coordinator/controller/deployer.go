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
	realmService *service.RealmService
	authService  *service.AuthService
}

// NewDeployerController creates a new DeployerController.
func NewDeployerController(
	realmService *service.RealmService,
	authService *service.AuthService,
) *DeployerController {
	return &DeployerController{
		realmService: realmService,
		authService:  authService,
	}
}

// HandleDeployerJoin handles POST /api/v1/deployer/join requests.
func (c *DeployerController) HandleDeployerJoin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	apiKey := service.GetBearerToken(r)
	if apiKey == "" {
		http.Error(w, "authorization required", http.StatusUnauthorized)
		return
	}

	realm, err := c.authService.AuthenticateAPIKey(ctx, apiKey)
	if err != nil {
		slog.Error("authenticate API key", "error", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	authKey, err := c.realmService.CreateAuthKey(ctx, realm, 24*time.Hour, false)
	if err != nil {
		slog.Error("create auth key", "error", err)
		http.Error(w, "create auth key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(JoinCredentialsResponse{
		AuthKey:      authKey,
		HeadscaleURL: c.realmService.GetPublicURL(),
		User:         realm.HeadscaleUser,
	})
}
