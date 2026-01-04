package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/jwtauth"
)

var (
	ErrTokenExchange   = errors.New("token exchange failed")
	ErrInvalidIDToken  = errors.New("invalid ID token")
	ErrMissingAuthCode = errors.New("missing authorization code")
	ErrOAuthError      = errors.New("OAuth error from provider")
)

// OIDCConfig holds configuration for OIDC authentication.
type OIDCConfig struct {
	KeycloakURL    string
	Realm          string
	ClientID       string
	ClientSecret   string
	RedirectURI    string
	StateTTL       time.Duration
}

// OIDCService manages OIDC authentication flows.
type OIDCService struct {
	config       OIDCConfig
	jwtValidator *jwtauth.Validator

	stateMu sync.RWMutex
	states  map[string]stateEntry
}

type stateEntry struct {
	createdAt time.Time
}

// TokenResponse represents the OIDC token response from Keycloak.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// NewOIDCService creates a new OIDCService.
func NewOIDCService(config OIDCConfig, jwtValidator *jwtauth.Validator) *OIDCService {
	if config.StateTTL == 0 {
		config.StateTTL = 10 * time.Minute
	}

	svc := &OIDCService{
		config:       config,
		jwtValidator: jwtValidator,
		states:       make(map[string]stateEntry),
	}

	go svc.cleanupExpiredStates()

	return svc
}

// cleanupExpiredStates periodically removes expired state entries.
func (s *OIDCService) cleanupExpiredStates() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.stateMu.Lock()
		now := time.Now()
		for state, entry := range s.states {
			if now.Sub(entry.createdAt) > s.config.StateTTL {
				delete(s.states, state)
			}
		}
		s.stateMu.Unlock()
	}
}

// GenerateAuthURL generates the authorization URL for the OIDC login flow.
func (s *OIDCService) GenerateAuthURL() (authURL, state string, err error) {
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", fmt.Errorf("generate state: %w", err)
	}
	state = base64.RawURLEncoding.EncodeToString(stateBytes)

	s.stateMu.Lock()
	s.states[state] = stateEntry{createdAt: time.Now()}
	s.stateMu.Unlock()

	params := url.Values{}
	params.Set("client_id", s.config.ClientID)
	params.Set("redirect_uri", s.config.RedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "openid profile email")
	params.Set("state", state)

	authURL = fmt.Sprintf(
		"%s/realms/%s/protocol/openid-connect/auth?%s",
		s.config.KeycloakURL,
		s.config.Realm,
		params.Encode(),
	)

	return authURL, state, nil
}

// ValidateState checks if a state value is valid and not expired.
// The state is consumed (one-time use).
func (s *OIDCService) ValidateState(state string) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	entry, exists := s.states[state]
	if !exists {
		return false
	}

	delete(s.states, state)

	if time.Since(entry.createdAt) > s.config.StateTTL {
		return false
	}

	return true
}

// ExchangeCode exchanges an authorization code for tokens.
func (s *OIDCService) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	tokenURL := fmt.Sprintf(
		"%s/realms/%s/protocol/openid-connect/token",
		s.config.KeycloakURL,
		s.config.Realm,
	)

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", s.config.RedirectURI)
	data.Set("client_id", s.config.ClientID)
	data.Set("client_secret", s.config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status=%d body=%s", ErrTokenExchange, resp.StatusCode, string(body))
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
