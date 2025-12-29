package service

import (
	"context"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
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
	wonderNetManager *headscale.WonderNetManager
}

// NewNodesService creates a new NodesService.
func NewNodesService(wonderNetManager *headscale.WonderNetManager) *NodesService {
	return &NodesService{
		wonderNetManager: wonderNetManager,
	}
}

// ListNodes returns all nodes in the given wonder net.
func (s *NodesService) ListNodes(ctx context.Context, wonderNet *repository.WonderNet) ([]*Node, error) {
	nodes, err := s.wonderNetManager.GetWonderNetNodes(ctx, wonderNet.HeadscaleUser)
	if err != nil {
		return nil, err
	}

	result := make([]*Node, len(nodes))
	for i, node := range nodes {
		n := &Node{
			ID:      node.GetId(),
			Name:    node.GetName(),
			IPAddrs: node.GetIpAddresses(),
			Online:  node.GetOnline(),
		}
		if node.GetLastSeen() != nil {
			t := node.GetLastSeen().AsTime()
			n.LastSeen = &t
		}
		result[i] = n
	}

	return result, nil
}
