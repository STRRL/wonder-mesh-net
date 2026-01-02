package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/jwtauth"
	"github.com/strrl/wonder-mesh-net/pkg/meshbackend"
)

var (
	ErrNoWonderNet = errors.New("no wonder net found")
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
		// TODO: get MeshType from MeshBackend when multi-backend support is added
		MeshType: string(meshbackend.MeshTypeTailscale),
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

// GetWonderNetByOwner returns the first wonder net owned by a user.
func (s *WonderNetService) GetWonderNetByOwner(ctx context.Context, userID string) (*repository.WonderNet, error) {
	wonderNets, err := s.wonderNetRepository.ListByOwner(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list wonder nets by owner: %w", err)
	}
	if len(wonderNets) == 0 {
		return nil, nil
	}
	return wonderNets[0], nil
}

// GetOrCreateWonderNet gets an existing wonder net for a user or creates a new one.
// ownerID is the OIDC subject claim (user ID from IdP).
// displayName is used for the wonder net display name when creating a new one.
func (s *WonderNetService) GetOrCreateWonderNet(ctx context.Context, ownerID, displayName string) (*repository.WonderNet, error) {
	wonderNet, err := s.GetWonderNetByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if wonderNet != nil {
		if err := s.EnsureHeadscaleWonderNet(ctx, wonderNet.HeadscaleUser); err != nil {
			slog.Warn("ensure headscale wonder net", "error", err, "wonder_net_id", wonderNet.ID)
		}
		return wonderNet, nil
	}

	return s.ProvisionWonderNet(ctx, ownerID, displayName+"'s Wonder Net")
}

// ResolveWonderNetFromClaims returns the wonder net for a user based on JWT claims.
// It auto-creates a WonderNet if none exists for the user.
// Service account tokens are rejected since service account support was removed.
func (s *WonderNetService) ResolveWonderNetFromClaims(ctx context.Context, claims *jwtauth.Claims) (*repository.WonderNet, error) {
	if claims.IsServiceAccount() {
		return nil, fmt.Errorf("service account tokens are not supported")
	}

	displayName := claims.PreferredUsername
	if displayName == "" {
		displayName = claims.Name
	}
	if displayName == "" {
		displayName = claims.Email
	}
	return s.GetOrCreateWonderNet(ctx, claims.Subject, displayName)
}
