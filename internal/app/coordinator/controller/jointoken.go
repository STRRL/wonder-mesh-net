package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// JoinTokenController handles join token creation for workers.
type JoinTokenController struct {
	workerService *service.WorkerService
}

// NewJoinTokenController creates a new JoinTokenController.
func NewJoinTokenController(workerService *service.WorkerService) *JoinTokenController {
	return &JoinTokenController{
		workerService: workerService,
	}
}

// JoinTokenResponse represents the response body for creating a join token.
type JoinTokenResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
}

// HandleCreateJoinToken handles GET /api/v1/join-token requests.
// Creates a JWT join token for worker nodes.
func (c *JoinTokenController) HandleCreateJoinToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wonderNet := WonderNetFromContext(r)
	if wonderNet == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
