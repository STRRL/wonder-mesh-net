package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
)

// Authentication errors
var (
	ErrNoCredentials  = errors.New("no credentials provided")
	ErrInvalidSession = errors.New("invalid session")
	ErrInvalidAPIKey  = errors.New("invalid API key")
	ErrRealmNotFound  = errors.New("realm not found")
	ErrNoRealm        = errors.New("user has no realm")
)

// AuthHelper provides common authentication methods for handlers.
type AuthHelper struct {
	sessionRepository repository.SessionRepository
	realmRepository   repository.RealmRepository
}

// NewAuthHelper creates a new AuthHelper.
func NewAuthHelper(sessionRepository repository.SessionRepository, realmRepository repository.RealmRepository) *AuthHelper {
	return &AuthHelper{
		sessionRepository: sessionRepository,
		realmRepository:   realmRepository,
	}
}

// AuthenticateSession authenticates a request using X-Session-Token header.
// Returns the user's first realm.
func (h *AuthHelper) AuthenticateSession(r *http.Request) (*repository.Realm, error) {
	sessionID := r.Header.Get("X-Session-Token")
	if sessionID == "" {
		return nil, ErrNoCredentials
	}

	ctx := r.Context()

	session, err := h.sessionRepository.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrInvalidSession
	}

	realms, err := h.realmRepository.ListByOwner(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if len(realms) == 0 {
		return nil, ErrNoRealm
	}

	return realms[0], nil
}

// GetBearerToken extracts bearer token from Authorization header.
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

// GetRealmByID retrieves a realm by ID.
func (h *AuthHelper) GetRealmByID(ctx context.Context, realmID string) (*repository.Realm, error) {
	realm, err := h.realmRepository.Get(ctx, realmID)
	if err != nil {
		return nil, err
	}
	if realm == nil {
		return nil, ErrRealmNotFound
	}
	return realm, nil
}

// GetRealmFromRequest authenticates and gets user's first realm from header or cookie.
func (h *AuthHelper) GetRealmFromRequest(ctx context.Context, r *http.Request) (*repository.Realm, error) {
	sessionID := r.Header.Get("X-Session-Token")
	if sessionID == "" {
		cookie, err := r.Cookie("wonder_session")
		if err == nil && cookie.Value != "" {
			sessionID = cookie.Value
		}
	}

	if sessionID == "" {
		return nil, ErrNoCredentials
	}

	session, err := h.sessionRepository.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrInvalidSession
	}

	realms, err := h.realmRepository.ListByOwner(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if len(realms) == 0 {
		return nil, ErrNoRealm
	}

	return realms[0], nil
}
