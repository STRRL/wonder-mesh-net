package service

import (
	"context"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
)

// RealmService manages realm provisioning and Headscale integration.
type RealmService struct {
	realmRepository *repository.RealmRepository
	realmManager    *headscale.RealmManager
	aclManager      *headscale.ACLManager
	publicURL       string
}

// NewRealmService creates a new RealmService.
func NewRealmService(
	realmRepository *repository.RealmRepository,
	realmManager *headscale.RealmManager,
	aclManager *headscale.ACLManager,
	publicURL string,
) *RealmService {
	return &RealmService{
		realmRepository: realmRepository,
		realmManager:    realmManager,
		aclManager:      aclManager,
		publicURL:       publicURL,
	}
}

// ProvisionRealm creates a new realm for a user, including Headscale namespace.
func (s *RealmService) ProvisionRealm(ctx context.Context, userID, displayName string) (*repository.Realm, error) {
	realmID, hsUser := headscale.NewRealmIdentifiers()

	newRealm := &repository.Realm{
		ID:            realmID,
		OwnerID:       userID,
		HeadscaleUser: hsUser,
		DisplayName:   displayName,
	}

	if err := s.realmRepository.Create(ctx, newRealm); err != nil {
		return nil, err
	}

	hsUserObj, err := s.realmManager.GetOrCreateRealm(ctx, hsUser)
	if err != nil {
		return nil, err
	}

	if err := s.aclManager.AddRealmToPolicy(ctx, hsUserObj.GetName()); err != nil {
		return nil, err
	}

	return newRealm, nil
}

// EnsureHeadscaleRealm ensures the Headscale realm exists and ACL is configured.
func (s *RealmService) EnsureHeadscaleRealm(ctx context.Context, headscaleUser string) error {
	hsUserObj, err := s.realmManager.GetOrCreateRealm(ctx, headscaleUser)
	if err != nil {
		return err
	}

	return s.aclManager.AddRealmToPolicy(ctx, hsUserObj.GetName())
}

// CreateAuthKey creates a Headscale auth key for a realm.
func (s *RealmService) CreateAuthKey(ctx context.Context, realm *repository.Realm, ttl time.Duration, reusable bool) (string, error) {
	key, err := s.realmManager.CreateAuthKeyByName(ctx, realm.HeadscaleUser, ttl, reusable)
	if err != nil {
		return "", err
	}
	return key.GetKey(), nil
}

// GetPublicURL returns the public URL for the coordinator.
func (s *RealmService) GetPublicURL() string {
	return s.publicURL
}

// InitializeACLPolicy sets up the autogroup self policy for ACL.
func (s *RealmService) InitializeACLPolicy(ctx context.Context) error {
	return s.aclManager.SetAutogroupSelfPolicy(ctx)
}
