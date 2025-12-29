package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/jwtauth"
	"github.com/strrl/wonder-mesh-net/pkg/keycloak"
)

var (
	ErrNoWonderNet            = errors.New("no wonder net found")
	ErrServiceAccountNotFound = errors.New("service account not found")
)

// ServiceAccountDetails contains the details of a newly created service account.
type ServiceAccountDetails struct {
	ClientID     string
	ClientSecret string
}

// ServiceAccountInfo contains information about an existing service account.
type ServiceAccountInfo struct {
	ClientID    string
	Description string
}

// WonderNetService manages wonder net provisioning and Headscale integration.
type WonderNetService struct {
	wonderNetRepository      *repository.WonderNetRepository
	serviceAccountRepository *repository.ServiceAccountRepository
	wonderNetManager         *headscale.WonderNetManager
	aclManager               *headscale.ACLManager
	keycloakClient           *keycloak.AdminClient
	publicURL                string
}

// NewWonderNetService creates a new WonderNetService.
func NewWonderNetService(
	wonderNetRepository *repository.WonderNetRepository,
	serviceAccountRepository *repository.ServiceAccountRepository,
	wonderNetManager *headscale.WonderNetManager,
	aclManager *headscale.ACLManager,
	keycloakClient *keycloak.AdminClient,
	publicURL string,
) *WonderNetService {
	return &WonderNetService{
		wonderNetRepository:      wonderNetRepository,
		serviceAccountRepository: serviceAccountRepository,
		wonderNetManager:         wonderNetManager,
		aclManager:               aclManager,
		keycloakClient:           keycloakClient,
		publicURL:                publicURL,
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
// keycloakSub is used directly as the owner_id.
// displayName is used for the wonder net display name when creating a new one.
func (s *WonderNetService) GetOrCreateWonderNet(ctx context.Context, keycloakSub, displayName string) (*repository.WonderNet, error) {
	wonderNet, err := s.GetWonderNetByOwner(ctx, keycloakSub)
	if err != nil {
		return nil, err
	}
	if wonderNet != nil {
		if err := s.EnsureHeadscaleWonderNet(ctx, wonderNet.HeadscaleUser); err != nil {
			slog.Warn("ensure headscale wonder net", "error", err, "wonder_net_id", wonderNet.ID)
		}
		return wonderNet, nil
	}

	return s.ProvisionWonderNet(ctx, keycloakSub, displayName+"'s Wonder Net")
}

// ResolveWonderNetFromClaims returns the wonder net for a user or service account.
func (s *WonderNetService) ResolveWonderNetFromClaims(ctx context.Context, claims *jwtauth.Claims) (*repository.WonderNet, bool, error) {
	clientID, err := s.getServiceAccountClientID(ctx, claims.Subject)
	if err != nil {
		return nil, false, err
	}

	if clientID != "" {
		wonderNet, err := s.getServiceAccountWonderNetByClientID(ctx, clientID)
		if err != nil {
			return nil, true, err
		}
		return wonderNet, true, nil
	}

	displayName := claims.PreferredUsername
	if displayName == "" {
		displayName = claims.Name
	}
	if displayName == "" {
		displayName = claims.Email
	}
	wonderNet, err := s.GetOrCreateWonderNet(ctx, claims.Subject, displayName)
	if err != nil {
		return nil, false, err
	}
	return wonderNet, false, nil
}

// GetServiceAccountWonderNet returns the wonder net associated with a service account.
func (s *WonderNetService) GetServiceAccountWonderNet(ctx context.Context, claims *jwtauth.Claims) (*repository.WonderNet, error) {
	clientID, err := s.getServiceAccountClientID(ctx, claims.Subject)
	if err != nil {
		return nil, err
	}
	if clientID == "" {
		return nil, fmt.Errorf("not a service account")
	}
	return s.getServiceAccountWonderNetByClientID(ctx, clientID)
}

// CreateServiceAccount creates a new Keycloak service account for a wonder net.
func (s *WonderNetService) CreateServiceAccount(ctx context.Context, wonderNet *repository.WonderNet, name string) (*ServiceAccountDetails, error) {
	clientID := uuid.New().String()
	description := fmt.Sprintf("Service account for wonder net %s", wonderNet.ID)

	kcAccount, err := s.keycloakClient.CreateServiceAccount(ctx, clientID, description)
	if err != nil {
		return nil, fmt.Errorf("create keycloak service account: %w", err)
	}

	_, err = s.serviceAccountRepository.Create(ctx, wonderNet.ID, clientID, name)
	if err != nil {
		if cleanupErr := s.keycloakClient.DeleteServiceAccount(ctx, clientID); cleanupErr != nil {
			slog.Error("cleanup keycloak service account after db error", "error", cleanupErr, "client_id", clientID)
		}
		return nil, fmt.Errorf("store service account: %w", err)
	}

	slog.Info("created service account", "client_id", clientID, "wonder_net_id", wonderNet.ID)

	return &ServiceAccountDetails{
		ClientID:     clientID,
		ClientSecret: kcAccount.ClientSecret,
	}, nil
}

// ListServiceAccounts lists all service accounts for a wonder net.
func (s *WonderNetService) ListServiceAccounts(ctx context.Context, wonderNet *repository.WonderNet) ([]*ServiceAccountInfo, error) {
	accounts, err := s.serviceAccountRepository.ListByWonderNet(ctx, wonderNet.ID)
	if err != nil {
		return nil, fmt.Errorf("list service accounts: %w", err)
	}

	result := make([]*ServiceAccountInfo, len(accounts))
	for i, account := range accounts {
		result[i] = &ServiceAccountInfo{
			ClientID:    account.KeycloakClientID,
			Description: account.Name,
		}
	}
	return result, nil
}

// DeleteServiceAccount deletes a service account by client ID.
func (s *WonderNetService) DeleteServiceAccount(ctx context.Context, clientID string, wonderNet *repository.WonderNet) error {
	serviceAccount, err := s.serviceAccountRepository.GetByClientID(ctx, clientID)
	if err != nil {
		return fmt.Errorf("get service account: %w", err)
	}
	if serviceAccount == nil {
		return ErrServiceAccountNotFound
	}

	if serviceAccount.WonderNetID != wonderNet.ID {
		return ErrServiceAccountNotFound
	}

	if err := s.keycloakClient.DeleteServiceAccount(ctx, clientID); err != nil {
		slog.Error("delete keycloak service account", "error", err, "client_id", clientID)
	}

	if err := s.serviceAccountRepository.Delete(ctx, clientID); err != nil {
		return fmt.Errorf("delete service account: %w", err)
	}

	slog.Info("deleted service account", "client_id", clientID, "wonder_net_id", wonderNet.ID)
	return nil
}

func (s *WonderNetService) getServiceAccountClientID(ctx context.Context, keycloakSub string) (string, error) {
	if keycloakSub == "" {
		return "", fmt.Errorf("empty keycloak subject")
	}

	user, err := s.keycloakClient.GetUserByKeycloakSub(ctx, keycloakSub)
	if err != nil {
		return "", err
	}

	clientID := ""
	if user != nil && user.ServiceAccountClientID != nil {
		clientID = *user.ServiceAccountClientID
	}

	return clientID, nil
}

func (s *WonderNetService) getServiceAccountWonderNetByClientID(ctx context.Context, clientID string) (*repository.WonderNet, error) {
	serviceAccount, err := s.serviceAccountRepository.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("get service account: %w", err)
	}
	if serviceAccount == nil {
		return nil, ErrServiceAccountNotFound
	}

	wonderNet, err := s.wonderNetRepository.Get(ctx, serviceAccount.WonderNetID)
	if err != nil {
		return nil, fmt.Errorf("get wonder net: %w", err)
	}
	if wonderNet == nil {
		return nil, ErrNoWonderNet
	}

	return wonderNet, nil
}
