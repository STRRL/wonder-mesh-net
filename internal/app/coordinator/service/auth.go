package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
)

// Authentication errors returned by AuthService methods.
var (
	ErrNoCredentials   = errors.New("no credentials provided")
	ErrInvalidSession  = errors.New("invalid session")
	ErrInvalidAPIKey   = errors.New("invalid API key")
	ErrRealmNotFound   = errors.New("realm not found")
	ErrNoRealm         = errors.New("user has no realm")
	ErrSessionRequired = errors.New("session token required, API key not allowed")
	ErrAPIKeyRequired  = errors.New("API key required, session token not allowed")
)

// AuthService provides authentication and session management.
type AuthService struct {
	sessionRepository *repository.SessionRepository
	realmRepository   *repository.RealmRepository
	apiKeyRepository  *repository.APIKeyRepository
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	sessionRepository *repository.SessionRepository,
	realmRepository *repository.RealmRepository,
	apiKeyRepository *repository.APIKeyRepository,
) *AuthService {
	return &AuthService{
		sessionRepository: sessionRepository,
		realmRepository:   realmRepository,
		apiKeyRepository:  apiKeyRepository,
	}
}

// AuthenticateSession validates a session token and returns the user's realm.
func (s *AuthService) AuthenticateSession(ctx context.Context, sessionToken string) (*repository.Realm, error) {
	if sessionToken == "" {
		return nil, ErrNoCredentials
	}

	session, err := s.sessionRepository.Get(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrInvalidSession
	}

	realms, err := s.realmRepository.ListByOwner(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if len(realms) == 0 {
		return nil, ErrNoRealm
	}

	return realms[0], nil
}

// AuthenticateAPIKey validates an API key and returns the associated realm.
func (s *AuthService) AuthenticateAPIKey(ctx context.Context, apiKey string) (*repository.Realm, error) {
	if apiKey == "" {
		return nil, ErrNoCredentials
	}

	key, err := s.apiKeyRepository.GetByKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, ErrInvalidAPIKey
	}

	if err := s.apiKeyRepository.UpdateLastUsed(ctx, key.ID); err != nil {
		return nil, err
	}

	return s.GetRealmByID(ctx, key.RealmID)
}

// Authenticate tries to authenticate using a bearer token.
// The token can be either a session token or an API key - both are tried in order.
func (s *AuthService) Authenticate(ctx context.Context, token string) (*repository.Realm, error) {
	if token == "" {
		return nil, ErrNoCredentials
	}

	if realm, err := s.AuthenticateSession(ctx, token); err == nil {
		return realm, nil
	}

	return s.AuthenticateAPIKey(ctx, token)
}

// GetRealmByID retrieves a realm by its ID.
func (s *AuthService) GetRealmByID(ctx context.Context, realmID string) (*repository.Realm, error) {
	realm, err := s.realmRepository.Get(ctx, realmID)
	if err != nil {
		return nil, err
	}
	if realm == nil {
		return nil, ErrRealmNotFound
	}
	return realm, nil
}

// GetRealmFromRequest authenticates using Bearer token or cookie.
// Accepts both session tokens and API keys. Use SessionOnly or APIKeyOnly for restricted access.
func (s *AuthService) GetRealmFromRequest(ctx context.Context, r *http.Request) (*repository.Realm, error) {
	token := GetBearerToken(r)
	if token == "" {
		cookie, err := r.Cookie("wonder_session")
		if err == nil && cookie.Value != "" {
			token = cookie.Value
		}
	}

	return s.Authenticate(ctx, token)
}

// SessionOnly authenticates using Bearer token or cookie, but only accepts session tokens.
// Use this for privileged endpoints that should not be accessible via API keys.
func (s *AuthService) SessionOnly(ctx context.Context, r *http.Request) (*repository.Realm, error) {
	token := GetBearerToken(r)
	if token == "" {
		cookie, err := r.Cookie("wonder_session")
		if err == nil && cookie.Value != "" {
			token = cookie.Value
		}
	}

	if token == "" {
		return nil, ErrNoCredentials
	}

	return s.AuthenticateSession(ctx, token)
}

// APIKeyOnly authenticates using Bearer token, but only accepts API keys.
// Use this for endpoints designed specifically for third-party integrations.
func (s *AuthService) APIKeyOnly(ctx context.Context, r *http.Request) (*repository.Realm, error) {
	token := GetBearerToken(r)
	if token == "" {
		return nil, ErrNoCredentials
	}

	return s.AuthenticateAPIKey(ctx, token)
}

// CreateSession creates a new session for a user.
func (s *AuthService) CreateSession(ctx context.Context, userID string, ttl time.Duration) (*repository.Session, error) {
	sessionID, err := repository.GenerateSessionID()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(ttl)
	session := &repository.Session{
		ID:        sessionID,
		UserID:    userID,
		ExpiresAt: &expiresAt,
	}

	if err := s.sessionRepository.Create(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}

// GetBearerToken extracts a bearer token from the Authorization header.
func GetBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(authHeader, "Bearer ")
}
