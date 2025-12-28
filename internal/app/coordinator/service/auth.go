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
	ErrNoCredentials  = errors.New("no credentials provided")
	ErrInvalidSession = errors.New("invalid session")
	ErrInvalidAPIKey  = errors.New("invalid API key")
	ErrRealmNotFound  = errors.New("realm not found")
	ErrNoRealm        = errors.New("user has no realm")
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

// Authenticate tries API key first, then falls back to session token.
func (s *AuthService) Authenticate(ctx context.Context, sessionToken, apiKey string) (*repository.Realm, error) {
	if apiKey != "" {
		return s.AuthenticateAPIKey(ctx, apiKey)
	}
	return s.AuthenticateSession(ctx, sessionToken)
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

// GetRealmFromRequest authenticates using session header or cookie.
func (s *AuthService) GetRealmFromRequest(ctx context.Context, r *http.Request) (*repository.Realm, error) {
	sessionID := r.Header.Get("X-Session-Token")
	if sessionID == "" {
		cookie, err := r.Cookie("wonder_session")
		if err == nil && cookie.Value != "" {
			sessionID = cookie.Value
		}
	}

	return s.AuthenticateSession(ctx, sessionID)
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
