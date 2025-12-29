package headscale

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WonderNetManager manages wonder nets (users) in Headscale
type WonderNetManager struct {
	headscaleClient v1.HeadscaleServiceClient
}

// NewWonderNetManager creates a new WonderNetManager
func NewWonderNetManager(client v1.HeadscaleServiceClient) *WonderNetManager {
	return &WonderNetManager{headscaleClient: client}
}

// NewWonderNetIdentifiers generates a new wonder net ID and Headscale username.
// This should be called once when creating a new wonder net.
// Both use the same UUID - the wonder net ID is the full UUID, and the headscale
// user is also the same UUID (used as namespace name in Headscale).
func NewWonderNetIdentifiers() (wonderNetID string, headscaleUser string) {
	id := uuid.New().String()
	return id, id
}

// GetOrCreateWonderNet gets an existing wonder net or creates a new one by name
func (m *WonderNetManager) GetOrCreateWonderNet(ctx context.Context, wonderNetName string) (*v1.User, error) {
	listResp, err := m.headscaleClient.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	for _, u := range listResp.GetUsers() {
		if u.GetName() == wonderNetName {
			return u, nil
		}
	}

	createResp, err := m.headscaleClient.CreateUser(ctx, &v1.CreateUserRequest{Name: wonderNetName})
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return createResp.GetUser(), nil
}

// CreateAuthKey creates a pre-auth key for a wonder net by user ID
func (m *WonderNetManager) CreateAuthKey(ctx context.Context, userID uint64, ttl time.Duration, reusable bool) (*v1.PreAuthKey, error) {
	expiration := time.Now().Add(ttl)

	resp, err := m.headscaleClient.CreatePreAuthKey(ctx, &v1.CreatePreAuthKeyRequest{
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

// CreateAuthKeyByName creates a pre-auth key for a wonder net by username.
// This method ensures the user exists in Headscale before creating the key,
// making it resilient to Headscale restarts.
func (m *WonderNetManager) CreateAuthKeyByName(ctx context.Context, username string, ttl time.Duration, reusable bool) (*v1.PreAuthKey, error) {
	headscaleUser, err := m.GetOrCreateWonderNet(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("get/create wonder net: %w", err)
	}

	return m.CreateAuthKey(ctx, headscaleUser.GetId(), ttl, reusable)
}

// GetWonderNetNodes gets all nodes for a wonder net
func (m *WonderNetManager) GetWonderNetNodes(ctx context.Context, username string) ([]*v1.Node, error) {
	resp, err := m.headscaleClient.ListNodes(ctx, &v1.ListNodesRequest{User: username})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	return resp.GetNodes(), nil
}
