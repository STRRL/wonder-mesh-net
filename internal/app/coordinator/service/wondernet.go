package service

import (
	"context"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
)

// WonderNetService manages wonder net provisioning and Headscale integration.
type WonderNetService struct {
	wonderNetRepository *repository.WonderNetRepository
	wonderNetManager    *headscale.WonderNetManager
	aclManager          *headscale.ACLManager
	publicURL           string
}

// NewWonderNetService creates a new WonderNetService.
func NewWonderNetService(
	wonderNetRepository *repository.WonderNetRepository,
	wonderNetManager *headscale.WonderNetManager,
	aclManager *headscale.ACLManager,
	publicURL string,
) *WonderNetService {
	return &WonderNetService{
		wonderNetRepository: wonderNetRepository,
		wonderNetManager:    wonderNetManager,
		aclManager:          aclManager,
		publicURL:           publicURL,
	}
}

// ProvisionWonderNet creates a new wonder net for a user, including Headscale namespace.
func (s *WonderNetService) ProvisionWonderNet(ctx context.Context, userID, displayName string) (*repository.WonderNet, error) {
	wonderNetID, hsUser := headscale.NewWonderNetIdentifiers()

	newWonderNet := &repository.WonderNet{
		ID:            wonderNetID,
		OwnerID:       userID,
		HeadscaleUser: hsUser,
		DisplayName:   displayName,
	}

	if err := s.wonderNetRepository.Create(ctx, newWonderNet); err != nil {
		return nil, err
	}

	hsUserObj, err := s.wonderNetManager.GetOrCreateWonderNet(ctx, hsUser)
	if err != nil {
		return nil, err
	}

	if err := s.aclManager.AddWonderNetToPolicy(ctx, hsUserObj.GetName()); err != nil {
		return nil, err
	}

	return newWonderNet, nil
}

// EnsureHeadscaleWonderNet ensures the Headscale wonder net exists and ACL is configured.
func (s *WonderNetService) EnsureHeadscaleWonderNet(ctx context.Context, headscaleUser string) error {
	hsUserObj, err := s.wonderNetManager.GetOrCreateWonderNet(ctx, headscaleUser)
	if err != nil {
		return err
	}

	return s.aclManager.AddWonderNetToPolicy(ctx, hsUserObj.GetName())
}

// CreateAuthKey creates a Headscale auth key for a wonder net.
func (s *WonderNetService) CreateAuthKey(ctx context.Context, wonderNet *repository.WonderNet, ttl time.Duration, reusable bool) (string, error) {
	key, err := s.wonderNetManager.CreateAuthKeyByName(ctx, wonderNet.HeadscaleUser, ttl, reusable)
	if err != nil {
		return "", err
	}
	return key.GetKey(), nil
}

// GetPublicURL returns the public URL for the coordinator.
func (s *WonderNetService) GetPublicURL() string {
	return s.publicURL
}

// InitializeACLPolicy sets up the autogroup self policy for ACL.
func (s *WonderNetService) InitializeACLPolicy(ctx context.Context) error {
	return s.aclManager.SetAutogroupSelfPolicy(ctx)
}
