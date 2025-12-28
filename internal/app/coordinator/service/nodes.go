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
	realmManager *headscale.RealmManager
}

// NewNodesService creates a new NodesService.
func NewNodesService(realmManager *headscale.RealmManager) *NodesService {
	return &NodesService{
		realmManager: realmManager,
	}
}

// ListNodes returns all nodes in the given realm.
func (s *NodesService) ListNodes(ctx context.Context, realm *repository.Realm) ([]*Node, error) {
	nodes, err := s.realmManager.GetRealmNodes(ctx, realm.HeadscaleUser)
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
