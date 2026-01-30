package service

import (
	"context"
	"testing"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/meshbackend"
)

// mockMeshBackend implements meshbackend.MeshBackend for testing isolation.
type mockMeshBackend struct {
	nodes []*meshbackend.Node
}

func (m *mockMeshBackend) MeshType() meshbackend.MeshType {
	return meshbackend.MeshTypeTailscale
}

func (m *mockMeshBackend) CreateRealm(ctx context.Context, name string) error {
	return nil
}

func (m *mockMeshBackend) GetRealm(ctx context.Context, name string) (bool, error) {
	return true, nil
}

func (m *mockMeshBackend) CreateJoinCredentials(ctx context.Context, realmName string, opts meshbackend.JoinOptions) (map[string]any, error) {
	return nil, nil
}

func (m *mockMeshBackend) ListNodes(ctx context.Context, realmName string) ([]*meshbackend.Node, error) {
	// Simulate a backend that returns nodes with Realm populated
	// but might accidentally return nodes from other realms (defense-in-depth test)
	return m.nodes, nil
}

func (m *mockMeshBackend) GetNode(ctx context.Context, nodeID string) (*meshbackend.Node, error) {
	for _, n := range m.nodes {
		if n.ID == nodeID {
			return n, nil
		}
	}
	return nil, nil
}

func (m *mockMeshBackend) DeleteNode(ctx context.Context, nodeID string) error {
	return nil
}

func (m *mockMeshBackend) Healthy(ctx context.Context) error {
	return nil
}

func TestNodesService_ListNodes_IsolatesRealms(t *testing.T) {
	now := time.Now()
	userARealm := "user-a-realm"
	userBRealm := "user-b-realm"

	mockBackend := &mockMeshBackend{
		nodes: []*meshbackend.Node{
			{ID: "1", Name: "node-a1", Realm: userARealm, Online: true, LastSeen: &now},
			{ID: "2", Name: "node-a2", Realm: userARealm, Online: true, LastSeen: &now},
			{ID: "3", Name: "node-b1", Realm: userBRealm, Online: true, LastSeen: &now},
			{ID: "4", Name: "node-b2", Realm: userBRealm, Online: false, LastSeen: &now},
		},
	}

	svc := NewNodesService(mockBackend)

	wonderNetA := &repository.WonderNet{
		ID:            "wonder-net-a",
		OwnerID:       "user-a",
		HeadscaleUser: userARealm,
	}

	nodes, err := svc.ListNodes(context.Background(), wonderNetA)
	if err != nil {
		t.Fatalf("ListNodes returned error: %v", err)
	}

	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes for user A, got %d", len(nodes))
	}

	for _, node := range nodes {
		if node.Name != "node-a1" && node.Name != "node-a2" {
			t.Errorf("unexpected node returned: %s (should not see user B's nodes)", node.Name)
		}
	}

	wonderNetB := &repository.WonderNet{
		ID:            "wonder-net-b",
		OwnerID:       "user-b",
		HeadscaleUser: userBRealm,
	}

	nodesB, err := svc.ListNodes(context.Background(), wonderNetB)
	if err != nil {
		t.Fatalf("ListNodes returned error: %v", err)
	}

	if len(nodesB) != 2 {
		t.Errorf("expected 2 nodes for user B, got %d", len(nodesB))
	}

	for _, node := range nodesB {
		if node.Name != "node-b1" && node.Name != "node-b2" {
			t.Errorf("unexpected node returned: %s (should not see user A's nodes)", node.Name)
		}
	}
}

func TestNodesService_ListNodes_EmptyWhenNoMatchingRealm(t *testing.T) {
	now := time.Now()
	mockBackend := &mockMeshBackend{
		nodes: []*meshbackend.Node{
			{ID: "1", Name: "node-a1", Realm: "other-realm", Online: true, LastSeen: &now},
		},
	}

	svc := NewNodesService(mockBackend)

	wonderNet := &repository.WonderNet{
		ID:            "wonder-net-x",
		OwnerID:       "user-x",
		HeadscaleUser: "user-x-realm",
	}

	nodes, err := svc.ListNodes(context.Background(), wonderNet)
	if err != nil {
		t.Fatalf("ListNodes returned error: %v", err)
	}

	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes for non-matching realm, got %d", len(nodes))
	}
}

func TestNodesService_GetNode_RejectsWrongRealm(t *testing.T) {
	now := time.Now()
	mockBackend := &mockMeshBackend{
		nodes: []*meshbackend.Node{
			{ID: "1", Name: "node-a1", Realm: "user-a-realm", Online: true, LastSeen: &now},
		},
	}

	svc := NewNodesService(mockBackend)

	_, err := svc.GetNode(context.Background(), "user-b-realm", "1")
	if err == nil {
		t.Error("expected error when accessing node from wrong realm, got nil")
	}
}

func TestNodesService_DeleteNode_RejectsWrongRealm(t *testing.T) {
	now := time.Now()
	mockBackend := &mockMeshBackend{
		nodes: []*meshbackend.Node{
			{ID: "1", Name: "node-a1", Realm: "user-a-realm", Online: true, LastSeen: &now},
		},
	}

	svc := NewNodesService(mockBackend)

	err := svc.DeleteNode(context.Background(), "user-b-realm", "1")
	if err == nil {
		t.Error("expected error when deleting node from wrong realm, got nil")
	}
}
