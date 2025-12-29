package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/jwtauth"
	"github.com/strrl/wonder-mesh-net/pkg/keycloak"
)

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrWonderNetExists  = errors.New("wonder net already exists")
	ErrNoWonderNet      = errors.New("no wonder net associated with user")
)

// KeycloakAuthService handles Keycloak-based authentication.
type KeycloakAuthService struct {
	keycloakClient           *keycloak.AdminClient
	userRepository           *repository.UserRepository
	wonderNetRepository      *repository.WonderNetRepository
	serviceAccountRepository *repository.ServiceAccountRepository
	wonderNetService         *WonderNetService
}

// NewKeycloakAuthService creates a new KeycloakAuthService.
func NewKeycloakAuthService(
	keycloakClient *keycloak.AdminClient,
	userRepository *repository.UserRepository,
	wonderNetRepository *repository.WonderNetRepository,
	serviceAccountRepository *repository.ServiceAccountRepository,
	wonderNetService *WonderNetService,
) *KeycloakAuthService {
	return &KeycloakAuthService{
		keycloakClient:           keycloakClient,
		userRepository:           userRepository,
		wonderNetRepository:      wonderNetRepository,
		serviceAccountRepository: serviceAccountRepository,
		wonderNetService:         wonderNetService,
	}
}

// EnsureUserAndWonderNet ensures a user and their wonder net exist for the given JWT claims.
// If the user doesn't exist, they are created along with their wonder net.
// Returns the user and wonder net.
func (s *KeycloakAuthService) EnsureUserAndWonderNet(ctx context.Context, claims *jwtauth.Claims) (*repository.User, *repository.WonderNet, error) {
	keycloakSub := claims.Subject
	if keycloakSub == "" {
		return nil, nil, fmt.Errorf("missing subject claim")
	}

	user, err := s.userRepository.GetByKeycloakSub(ctx, keycloakSub)
	if err != nil {
		return nil, nil, fmt.Errorf("get user by keycloak sub: %w", err)
	}

	if user == nil {
		displayName := claims.PreferredUsername
		if displayName == "" {
			displayName = claims.Name
		}
		if displayName == "" {
			displayName = claims.Email
		}

		user, err = s.userRepository.Create(ctx, keycloakSub, displayName)
		if err != nil {
			return nil, nil, fmt.Errorf("create user: %w", err)
		}
		slog.Info("created user from Keycloak", "user_id", user.ID, "display_name", displayName)

		wonderNet, err := s.wonderNetService.ProvisionWonderNet(ctx, user.ID, displayName+"'s Wonder Net")
		if err != nil {
			return nil, nil, fmt.Errorf("provision wonder net for new user: %w", err)
		}
		slog.Info("created wonder net for user", "user_id", user.ID, "wonder_net_id", wonderNet.ID)

		return user, wonderNet, nil
	}

	wonderNets, err := s.wonderNetRepository.ListByOwner(ctx, user.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("list wonder nets by owner: %w", err)
	}

	if len(wonderNets) == 0 {
		wonderNet, err := s.wonderNetService.ProvisionWonderNet(ctx, user.ID, user.DisplayName+"'s Wonder Net")
		if err != nil {
			return nil, nil, fmt.Errorf("provision wonder net for existing user: %w", err)
		}
		slog.Info("created wonder net for existing user", "user_id", user.ID, "wonder_net_id", wonderNet.ID)
		return user, wonderNet, nil
	}

	return user, wonderNets[0], nil
}

// GetUserWonderNet gets the wonder net for a user identified by JWT claims.
// Returns an error if the user or wonder net doesn't exist.
func (s *KeycloakAuthService) GetUserWonderNet(ctx context.Context, claims *jwtauth.Claims) (*repository.WonderNet, error) {
	keycloakSub := claims.Subject
	if keycloakSub == "" {
		return nil, fmt.Errorf("missing subject claim")
	}

	user, err := s.userRepository.GetByKeycloakSub(ctx, keycloakSub)
	if err != nil {
		return nil, fmt.Errorf("get user by keycloak sub: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	wonderNets, err := s.wonderNetRepository.ListByOwner(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("list wonder nets by owner: %w", err)
	}

	if len(wonderNets) == 0 {
		return nil, ErrNoWonderNet
	}

	return wonderNets[0], nil
}

// GetServiceAccountWonderNet gets the wonder net for a service account.
// Service accounts store their wonder net ID in a Keycloak user attribute.
func (s *KeycloakAuthService) GetServiceAccountWonderNet(ctx context.Context, claims *jwtauth.Claims) (*repository.WonderNet, error) {
	keycloakSub := claims.Subject
	if keycloakSub == "" {
		return nil, fmt.Errorf("missing subject claim")
	}

	wonderNetID, err := s.keycloakClient.GetUserAttribute(ctx, keycloakSub, "wonder_net_id")
	if err != nil {
		return nil, fmt.Errorf("get wonder net ID attribute: %w", err)
	}
	if wonderNetID == "" {
		return nil, ErrNoWonderNet
	}

	wonderNet, err := s.wonderNetRepository.Get(ctx, wonderNetID)
	if err != nil {
		return nil, fmt.Errorf("get wonder net: %w", err)
	}
	if wonderNet == nil {
		return nil, ErrNoWonderNet
	}

	return wonderNet, nil
}

// ServiceAccountDetails contains information about a created service account.
type ServiceAccountDetails struct {
	ClientID     string
	ClientSecret string
}

// CreateServiceAccount creates a Keycloak service account for a wonder net.
func (s *KeycloakAuthService) CreateServiceAccount(ctx context.Context, wonderNet *repository.WonderNet, name string) (*ServiceAccountDetails, error) {
	clientID := fmt.Sprintf("wonder-net-%s-%s", wonderNet.ID[:12], name)

	serviceAccount, err := s.keycloakClient.CreateServiceAccount(ctx, clientID, fmt.Sprintf("Service account for %s", name))
	if err != nil {
		return nil, fmt.Errorf("create service account: %w", err)
	}

	if err := s.keycloakClient.SetUserAttribute(ctx, serviceAccount.UserID, "wonder_net_id", wonderNet.ID); err != nil {
		if deleteErr := s.keycloakClient.DeleteServiceAccount(ctx, clientID); deleteErr != nil {
			slog.Error("cleanup service account after attribute set failure",
				"error", deleteErr,
				"client_id", clientID)
		}
		return nil, fmt.Errorf("set wonder net ID attribute: %w", err)
	}

	if _, err := s.serviceAccountRepository.Create(ctx, wonderNet.ID, clientID, name); err != nil {
		if deleteErr := s.keycloakClient.DeleteServiceAccount(ctx, clientID); deleteErr != nil {
			slog.Error("cleanup service account after database insert failure",
				"error", deleteErr,
				"client_id", clientID)
		}
		return nil, fmt.Errorf("store service account mapping: %w", err)
	}

	return &ServiceAccountDetails{
		ClientID:     serviceAccount.ClientID,
		ClientSecret: serviceAccount.ClientSecret,
	}, nil
}

var ErrServiceAccountNotFound = errors.New("service account not found")

// DeleteServiceAccount deletes a Keycloak service account.
// It verifies ownership via the database mapping before deleting.
func (s *KeycloakAuthService) DeleteServiceAccount(ctx context.Context, clientID string, wonderNet *repository.WonderNet) error {
	sa, err := s.serviceAccountRepository.GetByClientID(ctx, clientID)
	if err != nil {
		return fmt.Errorf("get service account: %w", err)
	}
	if sa == nil {
		return ErrServiceAccountNotFound
	}
	if sa.WonderNetID != wonderNet.ID {
		return ErrServiceAccountNotFound
	}

	if err := s.keycloakClient.DeleteServiceAccount(ctx, clientID); err != nil {
		return fmt.Errorf("delete keycloak service account: %w", err)
	}

	if err := s.serviceAccountRepository.Delete(ctx, clientID); err != nil {
		slog.Error("delete service account from database after keycloak deletion",
			"error", err,
			"client_id", clientID)
	}

	return nil
}

// ServiceAccountInfo represents a service account returned from list.
type ServiceAccountInfo struct {
	ClientID    string
	Name        string
	Description string
}

// ListServiceAccounts lists all service accounts for a wonder net.
func (s *KeycloakAuthService) ListServiceAccounts(ctx context.Context, wonderNet *repository.WonderNet) ([]*ServiceAccountInfo, error) {
	accounts, err := s.serviceAccountRepository.ListByWonderNet(ctx, wonderNet.ID)
	if err != nil {
		return nil, fmt.Errorf("list service accounts: %w", err)
	}

	result := make([]*ServiceAccountInfo, len(accounts))
	for i, account := range accounts {
		result[i] = &ServiceAccountInfo{
			ClientID:    account.KeycloakClientID,
			Name:        account.Name,
			Description: fmt.Sprintf("Service account for %s", account.Name),
		}
	}

	return result, nil
}
