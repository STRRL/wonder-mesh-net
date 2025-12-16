package headscale

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RealmManager manages realms (users) in Headscale
type RealmManager struct {
	client v1.HeadscaleServiceClient
}

// NewRealmManager creates a new RealmManager
func NewRealmManager(client v1.HeadscaleServiceClient) *RealmManager {
	return &RealmManager{client: client}
}

// GenerateRealmID generates a random UUID for a new realm
func GenerateRealmID() string {
	return uuid.New().String()
}

// RealmName returns the Headscale user name for a realm
func RealmName(realmID string) string {
	return "realm-" + realmID[:12]
}

// GetOrCreateRealm gets an existing realm or creates a new one by name
func (rm *RealmManager) GetOrCreateRealm(ctx context.Context, realmName string) (*v1.User, error) {
	listResp, err := rm.client.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	for _, u := range listResp.GetUsers() {
		if u.GetName() == realmName {
			return u, nil
		}
	}

	createResp, err := rm.client.CreateUser(ctx, &v1.CreateUserRequest{Name: realmName})
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return createResp.GetUser(), nil
}

// EnsureUser ensures a user exists in Headscale by name, creating if needed
func (rm *RealmManager) EnsureUser(ctx context.Context, name string) (*v1.User, error) {
	listResp, err := rm.client.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	for _, u := range listResp.GetUsers() {
		if u.GetName() == name {
			return u, nil
		}
	}

	createResp, err := rm.client.CreateUser(ctx, &v1.CreateUserRequest{Name: name})
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return createResp.GetUser(), nil
}

// CreateAuthKey creates a pre-auth key for a realm by user ID
func (rm *RealmManager) CreateAuthKey(ctx context.Context, userID uint64, ttl time.Duration, reusable bool) (*v1.PreAuthKey, error) {
	expiration := time.Now().Add(ttl)

	resp, err := rm.client.CreatePreAuthKey(ctx, &v1.CreatePreAuthKeyRequest{
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

// CreateAuthKeyByName creates a pre-auth key for a realm by username.
// This method ensures the user exists in Headscale before creating the key,
// making it resilient to Headscale restarts.
func (rm *RealmManager) CreateAuthKeyByName(ctx context.Context, username string, ttl time.Duration, reusable bool) (*v1.PreAuthKey, error) {
	hsUser, err := rm.EnsureUser(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure user: %w", err)
	}

	return rm.CreateAuthKey(ctx, hsUser.GetId(), ttl, reusable)
}

// GetRealmNodes gets all nodes for a realm
func (rm *RealmManager) GetRealmNodes(ctx context.Context, username string) ([]*v1.Node, error) {
	resp, err := rm.client.ListNodes(ctx, &v1.ListNodesRequest{User: username})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	return resp.GetNodes(), nil
}
