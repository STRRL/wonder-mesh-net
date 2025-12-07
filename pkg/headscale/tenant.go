package headscale

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TenantManager manages tenants (users) in Headscale
type TenantManager struct {
	client v1.HeadscaleServiceClient
}

// NewTenantManager creates a new TenantManager
func NewTenantManager(client v1.HeadscaleServiceClient) *TenantManager {
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
func (tm *TenantManager) GetOrCreateTenant(ctx context.Context, issuer, subject string) (*v1.User, error) {
	tenantID := DeriveTenantID(issuer, subject)
	name := TenantName(tenantID)

	listResp, err := tm.client.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	for _, u := range listResp.GetUsers() {
		if u.GetName() == name {
			return u, nil
		}
	}

	createResp, err := tm.client.CreateUser(ctx, &v1.CreateUserRequest{Name: name})
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return createResp.GetUser(), nil
}

// CreateAuthKey creates a pre-auth key for a tenant
func (tm *TenantManager) CreateAuthKey(ctx context.Context, userID uint64, ttl time.Duration, reusable bool) (*v1.PreAuthKey, error) {
	expiration := time.Now().Add(ttl)

	resp, err := tm.client.CreatePreAuthKey(ctx, &v1.CreatePreAuthKeyRequest{
		User:       userID,
		Reusable:   reusable,
		Ephemeral:  false,
		Expiration: timestamppb.New(expiration),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create pre-auth key: %w", err)
	}

	return resp.GetPreAuthKey(), nil
}

// GetTenantNodes gets all nodes for a tenant
func (tm *TenantManager) GetTenantNodes(ctx context.Context, username string) ([]*v1.Node, error) {
	resp, err := tm.client.ListNodes(ctx, &v1.ListNodesRequest{User: username})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	return resp.GetNodes(), nil
}
