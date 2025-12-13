package headscale

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
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

// EnsureUser ensures a user exists in Headscale by name, creating if needed
func (tm *TenantManager) EnsureUser(ctx context.Context, name string) (*v1.User, error) {
	log.Printf("DEBUG: EnsureUser called for name=%s", name)
	listResp, err := tm.client.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	log.Printf("DEBUG: ListUsers returned %d users", len(listResp.GetUsers()))
	for _, u := range listResp.GetUsers() {
		log.Printf("DEBUG: Checking user: ID=%d, Name=%s", u.GetId(), u.GetName())
		if u.GetName() == name {
			log.Printf("DEBUG: Found existing user: ID=%d, Name=%s", u.GetId(), u.GetName())
			return u, nil
		}
	}

	log.Printf("DEBUG: User not found, creating: Name=%s", name)
	createResp, err := tm.client.CreateUser(ctx, &v1.CreateUserRequest{Name: name})
	if err != nil {
		log.Printf("DEBUG: CreateUser failed: %v", err)
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	createdUser := createResp.GetUser()
	log.Printf("DEBUG: CreateUser succeeded: ID=%d, Name=%s", createdUser.GetId(), createdUser.GetName())
	return createdUser, nil
}

// CreateAuthKey creates a pre-auth key for a tenant by user ID
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

// CreateAuthKeyByName creates a pre-auth key for a tenant by username.
// This method ensures the user exists in Headscale before creating the key,
// making it resilient to Headscale restarts.
func (tm *TenantManager) CreateAuthKeyByName(ctx context.Context, username string, ttl time.Duration, reusable bool) (*v1.PreAuthKey, error) {
	log.Printf("DEBUG: CreateAuthKeyByName called for username=%s", username)
	hsUser, err := tm.EnsureUser(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure user: %w", err)
	}
	log.Printf("DEBUG: EnsureUser returned user with ID=%d, Name=%s", hsUser.GetId(), hsUser.GetName())

	authKey, err := tm.CreateAuthKey(ctx, hsUser.GetId(), ttl, reusable)
	if err != nil {
		log.Printf("DEBUG: CreateAuthKey failed for userID=%d: %v", hsUser.GetId(), err)
		return nil, err
	}
	log.Printf("DEBUG: CreateAuthKey succeeded for userID=%d", hsUser.GetId())
	return authKey, nil
}

// GetTenantNodes gets all nodes for a tenant
func (tm *TenantManager) GetTenantNodes(ctx context.Context, username string) ([]*v1.Node, error) {
	resp, err := tm.client.ListNodes(ctx, &v1.ListNodesRequest{User: username})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	return resp.GetNodes(), nil
}
