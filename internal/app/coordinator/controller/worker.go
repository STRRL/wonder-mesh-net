package controller

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// WorkerController handles worker node registration.
type WorkerController struct {
	workerService *service.WorkerService
	authService   *service.AuthService
}

// NewWorkerController creates a new WorkerController.
func NewWorkerController(
	workerService *service.WorkerService,
	authService *service.AuthService,
) *WorkerController {
	return &WorkerController{
		workerService: workerService,
		authService:   authService,
	}
}

// HandleCreateJoinToken handles POST /api/v1/join-token requests.
func (c *WorkerController) HandleCreateJoinToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	sessionID := r.Header.Get("X-Session-Token")
	if sessionID == "" {
		http.Error(w, "session token required", http.StatusUnauthorized)
		return
	}

	realm, err := c.authService.AuthenticateSession(ctx, sessionID)
	if err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	var req struct {
		TTL string `json:"ttl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.TTL = "15m"
	}

	ttl := 15 * time.Minute
	if req.TTL != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			http.Error(w, "invalid TTL format", http.StatusBadRequest)
			return
		}
		ttl = parsed
	}

	token, err := c.workerService.GenerateJoinToken(ctx, realm, ttl)
	if err != nil {
		slog.Error("generate join token", "error", err)
		http.Error(w, "generate join token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"token":   token,
		"command": fmt.Sprintf("wonder worker join %s", token),
	}); err != nil {
		slog.Error("encode join token response", "error", err)
	}
}

// HandleWorkerJoin handles POST /api/v1/worker/join requests.
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
	if err := json.NewEncoder(w).Encode(JoinCredentialsResponse{
		AuthKey:      creds.AuthKey,
		HeadscaleURL: creds.HeadscaleURL,
		User:         creds.User,
	}); err != nil {
		slog.Error("encode worker join response", "error", err)
	}
}
