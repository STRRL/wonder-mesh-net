package service

import (
	"context"
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
		}

		if node.LastSeen != nil {
			n.LastSeen = node.LastSeen
		}
		result[i] = n
	}

	return result, nil
}
