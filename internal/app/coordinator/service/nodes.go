package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/meshbackend"
)

// Node represents a mesh network node.
type Node struct {
	ID       uint64
	Name     string
	IPAddrs  []string
	Online   bool
	LastSeen *time.Time
}

// NodesService handles node listing operations.
type NodesService struct {
	meshBackend meshbackend.MeshBackend
}

// NewNodesService creates a new NodesService.
func NewNodesService(meshBackend meshbackend.MeshBackend) *NodesService {
	return &NodesService{
		meshBackend: meshBackend,
	}
}

// ListNodes returns all nodes in the given wonder net.
func (s *NodesService) ListNodes(ctx context.Context, wonderNet *repository.WonderNet) ([]*Node, error) {
	nodes, err := s.meshBackend.ListNodes(ctx, wonderNet.HeadscaleUser)
	if err != nil {
		return nil, err
	}

	result := make([]*Node, len(nodes))
	for i, node := range nodes {
		n := &Node{
			Name:    node.Name,
			IPAddrs: node.Addresses,
			Online:  node.Online,
		}

		// Parse ID from string to uint64
		if id, err := strconv.ParseUint(node.ID, 10, 64); err == nil {
			n.ID = id
		} else {
			slog.Warn("parse node ID", "node_name", node.Name, "raw_id", node.ID, "error", err)
		}

		if node.LastSeen != nil {
			n.LastSeen = node.LastSeen
		}
		result[i] = n
	}

	return result, nil
}

// GetNode returns a single node from a wonder net.
// It verifies that the node belongs to the specified wonder net.
// headscaleUser is the Headscale user/namespace from the wonder net record.
func (s *NodesService) GetNode(ctx context.Context, headscaleUser string, nodeID string) (*Node, error) {
	node, err := s.meshBackend.GetNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("get node: %w", err)
	}

	if node.Realm != headscaleUser {
		return nil, fmt.Errorf("node does not belong to this wonder net")
	}

	n := &Node{
		Name:     node.Name,
		IPAddrs:  node.Addresses,
		Online:   node.Online,
		LastSeen: node.LastSeen,
	}

	if id, err := strconv.ParseUint(node.ID, 10, 64); err == nil {
		n.ID = id
	}

	return n, nil
}

// DeleteNode deletes a node from a wonder net.
// It verifies that the node belongs to the specified wonder net before deletion.
// headscaleUser is the Headscale user/namespace from the wonder net record.
func (s *NodesService) DeleteNode(ctx context.Context, headscaleUser string, nodeID string) error {
	node, err := s.meshBackend.GetNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("get node: %w", err)
	}

	if node.Realm != headscaleUser {
		return fmt.Errorf("node does not belong to this wonder net")
	}

	return s.meshBackend.DeleteNode(ctx, nodeID)
}
