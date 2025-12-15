package handlers

import (
	"fmt"
	"net/http"

	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	hsClient v1.HeadscaleServiceClient
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(hsClient v1.HeadscaleServiceClient) *HealthHandler {
	return &HealthHandler{hsClient: hsClient}
}

// ServeHTTP handles GET /health requests (readiness check).
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, err := h.hsClient.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		http.Error(w, "headscale unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintln(w, "ok")
}

// HandleLiveness handles GET /livez requests (liveness check).
func HandleLiveness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintln(w, "alive")
}
