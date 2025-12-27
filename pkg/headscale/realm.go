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
	headscaleClient v1.HeadscaleServiceClient
}

// NewRealmManager creates a new RealmManager
func NewRealmManager(client v1.HeadscaleServiceClient) *RealmManager {
	return &RealmManager{headscaleClient: client}
}

// NewRealmIdentifiers generates a new realm ID and Headscale username.
// This should be called once when creating a new realm.
// The headscale_user follows the pattern "realm-{first12CharsOfUUID}".
func NewRealmIdentifiers() (realmID string, headscaleUser string) {
	id := uuid.New().String()
	return id, "realm-" + id[:12]
}

// GetOrCreateRealm gets an existing realm or creates a new one by name
func (rm *RealmManager) GetOrCreateRealm(ctx context.Context, realmName string) (*v1.User, error) {
	listResp, err := rm.headscaleClient.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	for _, u := range listResp.GetUsers() {
		if u.GetName() == realmName {
			return u, nil
		}
	}

	createResp, err := rm.headscaleClient.CreateUser(ctx, &v1.CreateUserRequest{Name: realmName})
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return createResp.GetUser(), nil
}

// CreateAuthKey creates a pre-auth key for a realm by user ID
func (rm *RealmManager) CreateAuthKey(ctx context.Context, userID uint64, ttl time.Duration, reusable bool) (*v1.PreAuthKey, error) {
	expiration := time.Now().Add(ttl)

	resp, err := rm.headscaleClient.CreatePreAuthKey(ctx, &v1.CreatePreAuthKeyRequest{
		User:       userID,
		Reusable:   reusable,
		Ephemeral:  false,
		Expiration: timestamppb.New(expiration),
	})
	if err != nil {
		return nil, fmt.Errorf("create pre-auth key: %w", err)
	}

	return resp.GetPreAuthKey(), nil
}

// CreateAuthKeyByName creates a pre-auth key for a realm by username.
// This method ensures the user exists in Headscale before creating the key,
// making it resilient to Headscale restarts.
func (rm *RealmManager) CreateAuthKeyByName(ctx context.Context, username string, ttl time.Duration, reusable bool) (*v1.PreAuthKey, error) {
	headscaleUser, err := rm.GetOrCreateRealm(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("get/create realm: %w", err)
	}

	return rm.CreateAuthKey(ctx, headscaleUser.GetId(), ttl, reusable)
}

// GetRealmNodes gets all nodes for a realm
func (rm *RealmManager) GetRealmNodes(ctx context.Context, username string) ([]*v1.Node, error) {
	resp, err := rm.headscaleClient.ListNodes(ctx, &v1.ListNodesRequest{User: username})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	return resp.GetNodes(), nil
}
