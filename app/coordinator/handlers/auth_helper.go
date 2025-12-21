package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

// Authentication errors
var (
	ErrNoCredentials     = errors.New("no credentials provided")
	ErrInvalidSession    = errors.New("invalid session")
	ErrInvalidAPIKey     = errors.New("invalid API key")
	ErrInsufficientScope = errors.New("insufficient scope")
	ErrUserNotFound      = errors.New("user not found")
)

// AuthHelper provides common authentication methods for handlers.
type AuthHelper struct {
	sessionStore oidc.SessionStore
	userStore    oidc.UserStore
}

// NewAuthHelper creates a new AuthHelper.
func NewAuthHelper(sessionStore oidc.SessionStore, userStore oidc.UserStore) *AuthHelper {
	return &AuthHelper{
		sessionStore: sessionStore,
		userStore:    userStore,
	}
}

// AuthenticateSession authenticates a request using X-Session-Token header.
func (h *AuthHelper) AuthenticateSession(r *http.Request) (*oidc.User, error) {
	sessionID := r.Header.Get("X-Session-Token")
	if sessionID == "" {
		return nil, ErrNoCredentials
	}

	ctx := r.Context()

	session, err := h.sessionStore.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrInvalidSession
	}

	user, err := h.userStore.Get(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	return user, nil
}

// GetBearerToken extracts bearer token from Authorization header.
// Returns empty string if not present or not a Bearer token.
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

// GetUserByID retrieves a user by ID.
func (h *AuthHelper) GetUserByID(ctx context.Context, userID string) (*oidc.User, error) {
	user, err := h.userStore.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// GetUserFromRequest tries to authenticate user from header or cookie.
func (h *AuthHelper) GetUserFromRequest(ctx context.Context, r *http.Request) (*oidc.User, error) {
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

	session, err := h.sessionStore.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrInvalidSession
	}

	user, err := h.userStore.Get(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	return user, nil
}
