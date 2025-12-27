package headscale

import (
	"context"

	"google.golang.org/grpc/credentials"
)

var _ credentials.PerRPCCredentials = (*APIKeyCredentials)(nil)

// APIKeyCredentials implements credentials.PerRPCCredentials for API key authentication
type APIKeyCredentials struct {
	APIKey string
}

// GetRequestMetadata returns the API key as Bearer token in metadata
func (c *APIKeyCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + c.APIKey,
	}, nil
}

// RequireTransportSecurity returns false to allow insecure connections
func (c *APIKeyCredentials) RequireTransportSecurity() bool {
	return false
}
