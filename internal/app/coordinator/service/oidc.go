package service

import (
	"context"
	"net/url"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

// CallbackResult contains the result of completing an OIDC callback.
type CallbackResult struct {
	UserID        string
	HeadscaleUser string
	IsNewUser     bool
}

// OIDCService handles OIDC authentication flows.
type OIDCService struct {
	oidcRegistry       *oidc.Registry
	userRepository     *repository.UserRepository
	identityRepository *repository.OIDCIdentityRepository
	realmRepository    *repository.RealmRepository
	realmService       *RealmService
}

// NewOIDCService creates a new OIDCService.
func NewOIDCService(
	oidcRegistry *oidc.Registry,
	userRepository *repository.UserRepository,
	identityRepository *repository.OIDCIdentityRepository,
	realmRepository *repository.RealmRepository,
	realmService *RealmService,
) *OIDCService {
	return &OIDCService{
		oidcRegistry:       oidcRegistry,
		userRepository:     userRepository,
		identityRepository: identityRepository,
		realmRepository:    realmRepository,
		realmService:       realmService,
	}
}

// ListProviders returns available OIDC providers.
func (s *OIDCService) ListProviders() []string {
	return s.oidcRegistry.ListProviders()
}

// InitiateLogin starts the OIDC login flow.
func (s *OIDCService) InitiateLogin(ctx context.Context, providerName, redirectURI, publicURL string) (string, error) {
	provider, ok := s.oidcRegistry.GetProvider(providerName)
	if !ok {
		return "", ErrProviderNotFound
	}

	if redirectURI == "" {
		redirectURI = publicURL + "/coordinator/oidc/complete"
	} else if !isValidRedirectURI(publicURL, redirectURI) {
		return "", ErrInvalidRedirectURI
	}

	authState, err := s.oidcRegistry.CreateAuthState(ctx, redirectURI, providerName)
	if err != nil {
		return "", err
	}

	return provider.GetAuthURL(authState.State), nil
}

// CompleteCallback completes the OIDC authorization code flow.
func (s *OIDCService) CompleteCallback(ctx context.Context, code, state string) (*CallbackResult, string, error) {
	authState, ok := s.oidcRegistry.ValidateState(ctx, state)
	if !ok {
		return nil, "", ErrInvalidState
	}

	provider, ok := s.oidcRegistry.GetProvider(authState.ProviderName)
	if !ok {
		return nil, "", ErrProviderNotFound
	}

	userInfo, err := provider.ExchangeCode(ctx, code)
	if err != nil {
		return nil, "", err
	}

	existingIdentity, err := s.identityRepository.GetByIssuerSubject(ctx, provider.Issuer(), userInfo.Subject)
	if err != nil {
		return nil, "", err
	}

	var userID string
	var headscaleUserName string
	var isNewUser bool

	if existingIdentity != nil {
		userID = existingIdentity.UserID

		existingIdentity.Email = userInfo.Email
		existingIdentity.Name = userInfo.Name
		existingIdentity.Picture = userInfo.Picture
		_ = s.identityRepository.Update(ctx, existingIdentity)

		realms, err := s.realmRepository.ListByOwner(ctx, userID)
		if err != nil {
			return nil, "", err
		}
		if len(realms) > 0 {
			headscaleUserName = realms[0].HeadscaleUser
		}
	} else {
		isNewUser = true

		newUser, err := s.userRepository.Create(ctx, userInfo.Name)
		if err != nil {
			return nil, "", err
		}
		userID = newUser.ID

		newIdentity := &repository.OIDCIdentity{
			UserID:  userID,
			Issuer:  provider.Issuer(),
			Subject: userInfo.Subject,
			Email:   userInfo.Email,
			Name:    userInfo.Name,
			Picture: userInfo.Picture,
		}
		if err := s.identityRepository.Create(ctx, newIdentity); err != nil {
			return nil, "", err
		}

		realm, err := s.realmService.ProvisionRealm(ctx, userID, userInfo.Name+"'s Realm")
		if err != nil {
			return nil, "", err
		}
		headscaleUserName = realm.HeadscaleUser
	}

	if headscaleUserName != "" && !isNewUser {
		if err := s.realmService.EnsureHeadscaleRealm(ctx, headscaleUserName); err != nil {
			return nil, "", err
		}
	}

	return &CallbackResult{
		UserID:        userID,
		HeadscaleUser: headscaleUserName,
		IsNewUser:     isNewUser,
	}, authState.RedirectURI, nil
}

// isValidRedirectURI validates that the redirect URI is same-origin as publicURL.
func isValidRedirectURI(publicURL, redirectURI string) bool {
	parsedPublic, err := url.Parse(publicURL)
	if err != nil {
		return false
	}

	parsedRedirect, err := url.Parse(redirectURI)
	if err != nil {
		return false
	}

	return parsedRedirect.Scheme == parsedPublic.Scheme &&
		parsedRedirect.Host == parsedPublic.Host
}
