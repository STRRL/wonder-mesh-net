// Package tailscale implements the MeshBackend interface using Headscale as the
// control server for Tailscale clients.
package tailscale

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"github.com/strrl/wonder-mesh-net/pkg/meshbackend"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TailscaleMesh implements MeshBackend using Headscale as the control server.
type TailscaleMesh struct {
	client     v1.HeadscaleServiceClient
	controlURL string
}

// NewTailscaleMesh creates a new TailscaleMesh backend.
//
// Parameters:
//   - client: gRPC client for communicating with Headscale
//   - controlURL: the public URL of the Headscale control server that workers will connect to
func NewTailscaleMesh(client v1.HeadscaleServiceClient, controlURL string) *TailscaleMesh {
	return &TailscaleMesh{
		client:     client,
		controlURL: controlURL,
	}
}

// MeshType returns the mesh type identifier.
func (m *TailscaleMesh) MeshType() meshbackend.MeshType {
	return meshbackend.MeshTypeTailscale
}

// CreateRealm creates a Headscale user (namespace) for the realm.
// This method is idempotent - if the realm already exists, it returns nil.
func (m *TailscaleMesh) CreateRealm(ctx context.Context, name string) error {
	exists, err := m.GetRealm(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	_, err = m.client.CreateUser(ctx, &v1.CreateUserRequest{Name: name})
	if err != nil {
		return fmt.Errorf("create headscale user: %w", err)
	}
	return nil
}

// GetRealm checks if a Headscale user exists.
func (m *TailscaleMesh) GetRealm(ctx context.Context, name string) (bool, error) {
	resp, err := m.client.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		return false, fmt.Errorf("list headscale users: %w", err)
	}

	for _, u := range resp.GetUsers() {
		if u.GetName() == name {
			return true, nil
		}
	}
	return false, nil
}

// getOrCreateRealm ensures the realm exists, creating it if necessary.
func (m *TailscaleMesh) getOrCreateRealm(ctx context.Context, name string) (*v1.User, error) {
	resp, err := m.client.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		return nil, fmt.Errorf("list headscale users: %w", err)
	}

	for _, u := range resp.GetUsers() {
		if u.GetName() == name {
			return u, nil
		}
	}

	createResp, err := m.client.CreateUser(ctx, &v1.CreateUserRequest{Name: name})
	if err != nil {
		return nil, fmt.Errorf("create headscale user: %w", err)
	}

	return createResp.GetUser(), nil
}

// CreateJoinCredentials creates a Headscale PreAuthKey and returns Tailscale-specific metadata.
//
// The returned metadata contains:
//   - login_server: the Headscale control URL
//   - authkey: the PreAuthKey for tailscale up --authkey
//   - headscale_user: the Headscale user/namespace name
func (m *TailscaleMesh) CreateJoinCredentials(ctx context.Context, realmName string, opts meshbackend.JoinOptions) (map[string]any, error) {
	user, err := m.getOrCreateRealm(ctx, realmName)
	if err != nil {
		return nil, err
	}

	expiration := time.Now().Add(opts.TTL)
	keyResp, err := m.client.CreatePreAuthKey(ctx, &v1.CreatePreAuthKeyRequest{
		User:       user.GetId(),
		Reusable:   opts.Reusable,
		Ephemeral:  opts.Ephemeral,
		Expiration: timestamppb.New(expiration),
	})
	if err != nil {
		return nil, fmt.Errorf("create pre-auth key: %w", err)
	}

	return map[string]any{
		"login_server":   m.controlURL,
		"authkey":        keyResp.GetPreAuthKey().GetKey(),
		"headscale_user": realmName,
	}, nil
}

// ListNodes returns all nodes in a realm.
func (m *TailscaleMesh) ListNodes(ctx context.Context, realmName string) ([]*meshbackend.Node, error) {
	resp, err := m.client.ListNodes(ctx, &v1.ListNodesRequest{User: realmName})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	nodes := make([]*meshbackend.Node, 0, len(resp.GetNodes()))
	for _, n := range resp.GetNodes() {
		node := &meshbackend.Node{
			ID:        fmt.Sprintf("%d", n.GetId()),
			Name:      n.GetName(),
			Addresses: n.GetIpAddresses(),
			Online:    n.GetOnline(),
		}
		if n.GetLastSeen() != nil {
			t := n.GetLastSeen().AsTime()
			node.LastSeen = &t
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetNode retrieves a single node by its ID.
func (m *TailscaleMesh) GetNode(ctx context.Context, nodeID string) (*meshbackend.Node, error) {
	var id uint64
	if _, err := fmt.Sscanf(nodeID, "%d", &id); err != nil {
		return nil, fmt.Errorf("parse node ID: %w", err)
	}

	resp, err := m.client.GetNode(ctx, &v1.GetNodeRequest{NodeId: id})
	if err != nil {
		return nil, fmt.Errorf("get node: %w", err)
	}

	hsNode := resp.GetNode()
	node := &meshbackend.Node{
		ID:        fmt.Sprintf("%d", hsNode.GetId()),
		Name:      hsNode.GetName(),
		Addresses: hsNode.GetIpAddresses(),
		Online:    hsNode.GetOnline(),
	}

	if hsNode.GetLastSeen() != nil {
		t := hsNode.GetLastSeen().AsTime()
		node.LastSeen = &t
	}

	// Store the realm (Headscale user) in a custom field
	// This is needed for verification in DeleteNode
	if hsNode.GetUser() != nil {
		node.Realm = hsNode.GetUser().GetName()
	}

	return node, nil
}

// DeleteNode removes a node from the mesh network.
func (m *TailscaleMesh) DeleteNode(ctx context.Context, nodeID string) error {
	// Convert nodeID from string to uint64
	var id uint64
	if _, err := fmt.Sscanf(nodeID, "%d", &id); err != nil {
		return fmt.Errorf("parse node ID: %w", err)
	}

	_, err := m.client.DeleteNode(ctx, &v1.DeleteNodeRequest{NodeId: id})
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	return nil
}

// Healthy checks if the Headscale server is reachable.
func (m *TailscaleMesh) Healthy(ctx context.Context) error {
	_, err := m.client.ListUsers(ctx, &v1.ListUsersRequest{})
	if err != nil {
		return fmt.Errorf("headscale health check: %w", err)
	}
	return nil
}
