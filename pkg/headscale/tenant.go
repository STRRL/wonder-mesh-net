package headscale

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// TenantManager manages tenants (users) in Headscale
type TenantManager struct {
	client *Client
}

// NewTenantManager creates a new TenantManager
func NewTenantManager(client *Client) *TenantManager {
	return &TenantManager{client: client}
}

// DeriveTenantID derives a tenant ID from OIDC issuer and subject
func DeriveTenantID(issuer, subject string) string {
	h := sha256.Sum256([]byte(issuer + "|" + subject))
	return hex.EncodeToString(h[:16])
}

// TenantName returns the Headscale user name for a tenant
func TenantName(tenantID string) string {
	return "tenant-" + tenantID[:12]
}

// GetOrCreateTenant gets an existing tenant or creates a new one
func (tm *TenantManager) GetOrCreateTenant(ctx context.Context, issuer, subject string) (*User, error) {
	tenantID := DeriveTenantID(issuer, subject)
	name := TenantName(tenantID)

	user, err := tm.client.GetUser(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if user != nil {
		return user, nil
	}

	user, err = tm.client.CreateUser(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// CreateAuthKey creates a pre-auth key for a tenant
func (tm *TenantManager) CreateAuthKey(ctx context.Context, username string, ttl time.Duration, reusable bool) (*PreAuthKey, error) {
	expiration := time.Now().Add(ttl)

	key, err := tm.client.CreatePreAuthKey(ctx, &CreatePreAuthKeyRequest{
		User:       username,
		Reusable:   reusable,
		Ephemeral:  false,
		Expiration: expiration,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create pre-auth key: %w", err)
	}

	return key, nil
}

// GetTenantNodes gets all nodes for a tenant
func (tm *TenantManager) GetTenantNodes(ctx context.Context, userID uint64) ([]*Node, error) {
	return tm.client.ListNodes(ctx, &userID)
}
