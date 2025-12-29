package controller

import (
	"net/http"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
)

// contextKey is a type for context keys used by controllers.
type contextKey string

// Context keys for request context values.
const (
	ContextKeyWonderNet contextKey = "wonder_net"
)

// WonderNetFromContext retrieves the WonderNet from the request context.
// This expects the middleware to have set the wonder net in the context.
func WonderNetFromContext(r *http.Request) *repository.WonderNet {
	if wn, ok := r.Context().Value(ContextKeyWonderNet).(*repository.WonderNet); ok {
		return wn
	}
	return nil
}
