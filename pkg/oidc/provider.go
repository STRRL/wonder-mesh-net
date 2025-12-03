package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ProviderConfig is the configuration for an OIDC provider
type ProviderConfig struct {
	Name         string   `json:"name"`
	Issuer       string   `json:"issuer"`
	ClientID     string   `json:"clientId"`
	ClientSecret string   `json:"clientSecret"`
	Scopes       []string `json:"scopes,omitempty"`
}

// Provider represents an OIDC provider
type Provider struct {
	config           ProviderConfig
	authEndpoint     string
	tokenEndpoint    string
	userinfoEndpoint string
}

// WellKnownConfig represents the OIDC discovery document
type WellKnownConfig struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserinfoEndpoint      string   `json:"userinfo_endpoint"`
	JwksURI               string   `json:"jwks_uri"`
	ScopesSupported       []string `json:"scopes_supported"`
}

// NewProvider creates a new OIDC provider from config
func NewProvider(ctx context.Context, config ProviderConfig) (*Provider, error) {
	wellKnownURL := strings.TrimSuffix(config.Issuer, "/") + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch well-known config: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch well-known config: status %d, body: %s", resp.StatusCode, string(body))
	}

	var wellKnown WellKnownConfig
	if err := json.NewDecoder(resp.Body).Decode(&wellKnown); err != nil {
		return nil, fmt.Errorf("failed to decode well-known config: %w", err)
	}

	if config.Scopes == nil {
		config.Scopes = []string{"openid", "profile", "email"}
	}

	return &Provider{
		config:           config,
		authEndpoint:     wellKnown.AuthorizationEndpoint,
		tokenEndpoint:    wellKnown.TokenEndpoint,
		userinfoEndpoint: wellKnown.UserinfoEndpoint,
	}, nil
}

// AuthState represents the state for an auth flow
type AuthState struct {
	State       string
	Nonce       string
	RedirectURI string
	CreatedAt   time.Time
}

// GenerateState generates a random state string
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// GetAuthURL returns the authorization URL for this provider
func (p *Provider) GetAuthURL(redirectURI, state string) string {
	params := url.Values{}
	params.Set("client_id", p.config.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", strings.Join(p.config.Scopes, " "))
	params.Set("state", state)

	return p.authEndpoint + "?" + params.Encode()
}

// TokenResponse represents the token response from the OIDC provider
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// ExchangeCode exchanges an authorization code for tokens
func (p *Provider) ExchangeCode(ctx context.Context, code, redirectURI string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", p.config.ClientID)
	data.Set("client_secret", p.config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// UserInfo represents user information from the OIDC provider
type UserInfo struct {
	Subject       string `json:"sub"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified,omitempty"`
	Name          string `json:"name,omitempty"`
	Picture       string `json:"picture,omitempty"`
}

// GetUserInfo fetches user info using an access token
func (p *Provider) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userinfoEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch user info: status %d, body: %s", resp.StatusCode, string(body))
	}

	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	return &userInfo, nil
}

// Name returns the provider name
func (p *Provider) Name() string {
	return p.config.Name
}

// Issuer returns the provider issuer
func (p *Provider) Issuer() string {
	return p.config.Issuer
}

// Registry manages multiple OIDC providers
type Registry struct {
	providers map[string]*Provider
	states    map[string]*AuthState
	mu        sync.RWMutex
	stateTTL  time.Duration
}

// NewRegistry creates a new provider registry
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]*Provider),
		states:    make(map[string]*AuthState),
		stateTTL:  10 * time.Minute,
	}
}

// RegisterProvider registers an OIDC provider
func (r *Registry) RegisterProvider(ctx context.Context, config ProviderConfig) error {
	provider, err := NewProvider(ctx, config)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.providers[config.Name] = provider
	r.mu.Unlock()

	return nil
}

// GetProvider gets a provider by name
func (r *Registry) GetProvider(name string) (*Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, ok := r.providers[name]
	return provider, ok
}

// ListProviders returns all provider names
func (r *Registry) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// CreateAuthState creates a new auth state
func (r *Registry) CreateAuthState(redirectURI string) (*AuthState, error) {
	state, err := GenerateState()
	if err != nil {
		return nil, err
	}

	nonce, err := GenerateState()
	if err != nil {
		return nil, err
	}

	authState := &AuthState{
		State:       state,
		Nonce:       nonce,
		RedirectURI: redirectURI,
		CreatedAt:   time.Now(),
	}

	r.mu.Lock()
	r.states[state] = authState
	r.mu.Unlock()

	return authState, nil
}

// ValidateState validates and consumes an auth state
func (r *Registry) ValidateState(state string) (*AuthState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	authState, ok := r.states[state]
	if !ok {
		return nil, false
	}

	if time.Since(authState.CreatedAt) > r.stateTTL {
		delete(r.states, state)
		return nil, false
	}

	delete(r.states, state)
	return authState, true
}

// CleanupExpiredStates removes expired auth states
func (r *Registry) CleanupExpiredStates() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for state, authState := range r.states {
		if now.Sub(authState.CreatedAt) > r.stateTTL {
			delete(r.states, state)
		}
	}
}
