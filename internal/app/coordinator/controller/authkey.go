package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// AuthKeyController handles Headscale auth key creation.
type AuthKeyController struct {
	wonderNetService *service.WonderNetService
}

// NewAuthKeyController creates a new AuthKeyController.
func NewAuthKeyController(wonderNetService *service.WonderNetService) *AuthKeyController {
	return &AuthKeyController{
		wonderNetService: wonderNetService,
	}
}

// AuthKeyRequest represents the request body for creating an auth key.
type AuthKeyRequest struct {
	TTLHours int  `json:"ttl_hours,omitempty"`
	Reusable bool `json:"reusable,omitempty"`
}

// HandleCreateAuthKey handles POST /api/v1/authkey requests.
// Creates a Headscale auth key for the authenticated user's wonder net.
func (c *AuthKeyController) HandleCreateAuthKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wonderNet := WonderNetFromContext(r)
	if wonderNet == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req AuthKeyRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}

	ttl := 24 * time.Hour
	if req.TTLHours > 0 {
		ttl = time.Duration(req.TTLHours) * time.Hour
	}

	authKey, err := c.wonderNetService.CreateAuthKey(r.Context(), wonderNet, ttl, req.Reusable)
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
