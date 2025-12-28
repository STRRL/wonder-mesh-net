package controller

import (
	"fmt"
	"net/http"

	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
)

// HealthController provides readiness checks for the coordinator service.
type HealthController struct {
	headscaleClient v1.HeadscaleServiceClient
}

// NewHealthController creates a new HealthController.
func NewHealthController(headscaleClient v1.HeadscaleServiceClient) *HealthController {
	return &HealthController{headscaleClient: headscaleClient}
}

// ServeHTTP handles GET /health requests.
func (c *HealthController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, err := c.headscaleClient.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		http.Error(w, "headscale unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintln(w, "ok")
}
