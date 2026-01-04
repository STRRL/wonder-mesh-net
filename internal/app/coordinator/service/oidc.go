package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/jwtauth"
)

const (
	stateLength       = 32
	stateTTL          = 10 * time.Minute
	sessionCookieName = "wonder_session"
	sessionTTL        = 24 * time.Hour
	cleanupInterval   = 5 * time.Minute
)

type OIDCConfig struct {
	KeycloakURL  string
	Realm        string
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// TokenResponse represents the response from the token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token"`
}

// SessionData holds session information stored on the server side.
type SessionData struct {
	UserID       string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// OIDCService handles OIDC authentication flow.
type OIDCService struct {
	config       OIDCConfig
	jwtValidator *jwtauth.Validator
	httpClient   *http.Client

	states  map[string]time.Time
	stateMu sync.RWMutex

	sessions  map[string]*SessionData
	sessionMu sync.RWMutex

	stopCleanup chan struct{}
}

func NewOIDCService(config OIDCConfig, jwtValidator *jwtauth.Validator) *OIDCService {
	s := &OIDCService{
		config:       config,
		jwtValidator: jwtValidator,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		states:      make(map[string]time.Time),
		sessions:    make(map[string]*SessionData),
		stopCleanup: make(chan struct{}),
	}
	go s.runCleanup()
	return s
}

func (s *OIDCService) runCleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.CleanupExpiredStates()
			s.CleanupExpiredSessions()
		case <-s.stopCleanup:
			return
		}
	}
}

func (s *OIDCService) Stop() {
	close(s.stopCleanup)
}

// GenerateAuthURL generates the Keycloak authorization URL with a new state parameter.
func (s *OIDCService) GenerateAuthURL() (string, string, error) {
	state, err := generateRandomString(stateLength)
	if err != nil {
		return "", "", fmt.Errorf("generate state: %w", err)
	}

	s.stateMu.Lock()
	s.states[state] = time.Now().Add(stateTTL)
	s.stateMu.Unlock()

	authURL := fmt.Sprintf(
		"%s/realms/%s/protocol/openid-connect/auth",
		s.config.KeycloakURL,
		s.config.Realm,
	)

	params := url.Values{}
	params.Set("client_id", s.config.ClientID)
	params.Set("response_type", "code")
	params.Set("scope", "openid profile email")
	params.Set("redirect_uri", s.config.RedirectURI)
	params.Set("state", state)

	return authURL + "?" + params.Encode(), state, nil
}

// ValidateState checks if the state parameter is valid and not expired.
func (s *OIDCService) ValidateState(state string) error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	expiresAt, exists := s.states[state]
	if !exists {
		return ErrInvalidState
	}

	delete(s.states, state)

	if time.Now().After(expiresAt) {
		return ErrStateExpired
	}

	return nil
}

// ExchangeCode exchanges the authorization code for tokens.
func (s *OIDCService) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	tokenURL := fmt.Sprintf(
		"%s/realms/%s/protocol/openid-connect/token",
		s.config.KeycloakURL,
		s.config.Realm,
	)

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", s.config.ClientID)
	data.Set("client_secret", s.config.ClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", s.config.RedirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d, body: %s", ErrTokenExchange, resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	return &tokenResp, nil
}

// ValidateIDToken validates the ID token and returns the claims.
func (s *OIDCService) ValidateIDToken(idToken string) (*jwtauth.Claims, error) {
	claims, err := s.jwtValidator.Validate(idToken)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidIDToken, err.Error())
	}
	return claims, nil
}

func (s *OIDCService) CreateSession(userID, accessToken, refreshToken string, expiresIn int) (string, time.Duration, error) {
	sessionID, err := generateRandomString(32)
	if err != nil {
		return "", 0, fmt.Errorf("generate session ID: %w", err)
	}

	ttl := sessionTTL
	if expiresIn > 0 {
		tokenTTL := time.Duration(expiresIn) * time.Second
		if tokenTTL < ttl {
			ttl = tokenTTL
		}
	}

	sessionHash := hashSessionID(sessionID)

	s.sessionMu.Lock()
	s.sessions[sessionHash] = &SessionData{
		UserID:       userID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(ttl),
	}
	s.sessionMu.Unlock()

	return sessionID, ttl, nil
}

// GetSession retrieves session data by session ID.
func (s *OIDCService) GetSession(sessionID string) (*SessionData, error) {
	sessionHash := hashSessionID(sessionID)

	s.sessionMu.RLock()
	session, exists := s.sessions[sessionHash]
	s.sessionMu.RUnlock()

	if !exists {
		return nil, ErrSessionNotFound
	}

	if time.Now().After(session.ExpiresAt) {
		s.sessionMu.Lock()
		delete(s.sessions, sessionHash)
		s.sessionMu.Unlock()
		return nil, ErrSessionExpired
	}

	return session, nil
}

// DeleteSession removes a session.
func (s *OIDCService) DeleteSession(sessionID string) {
	sessionHash := hashSessionID(sessionID)

	s.sessionMu.Lock()
	delete(s.sessions, sessionHash)
	s.sessionMu.Unlock()
}

// CleanupExpiredStates removes expired state entries.
func (s *OIDCService) CleanupExpiredStates() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	now := time.Now()
	for state, expiresAt := range s.states {
		if now.After(expiresAt) {
			delete(s.states, state)
		}
	}
}

// CleanupExpiredSessions removes expired session entries.
func (s *OIDCService) CleanupExpiredSessions() {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()

	now := time.Now()
	for sessionID, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, sessionID)
		}
	}
}

// GetSessionCookieName returns the name of the session cookie.
func (s *OIDCService) GetSessionCookieName() string {
	return sessionCookieName
}

func generateRandomString(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}
	bytesNeeded := (length*6 + 7) / 8
	bytes := make([]byte, bytesNeeded)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes)[:length], nil
}

// hashSessionID creates a SHA256 hash of the session ID for storage.
func hashSessionID(sessionID string) string {
	hash := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(hash[:])
}
