package handlers

import (
	"fmt"
	"net/http"

	"github.com/strrl/wonder-mesh-net/pkg/headscale"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	hsClient *headscale.Client
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(hsClient *headscale.Client) *HealthHandler {
	return &HealthHandler{hsClient: hsClient}
}

// ServeHTTP handles GET /health requests.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.hsClient.Health(ctx); err != nil {
		http.Error(w, "headscale unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintln(w, "ok")
}
