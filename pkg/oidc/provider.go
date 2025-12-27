package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	googleOAuth "golang.org/x/oauth2/google"
)

// UserInfo represents user information from any auth provider
type UserInfo struct {
	Subject       string `json:"sub"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified,omitempty"`
	Name          string `json:"name,omitempty"`
	Picture       string `json:"picture,omitempty"`
}

// Provider is the interface for all auth providers
type Provider interface {
	Name() string
	Issuer() string
	GetAuthURL(state string) string
	ExchangeCode(ctx context.Context, code string) (*UserInfo, error)
}

// ProviderConfig is the configuration for a provider
type ProviderConfig struct {
	Type         string   `json:"type"`
	Name         string   `json:"name"`
	Issuer       string   `json:"issuer,omitempty"`
	ClientID     string   `json:"clientId"`
	ClientSecret string   `json:"clientSecret"`
	Scopes       []string `json:"scopes,omitempty"`
	RedirectURL  string   `json:"redirectUrl"`
}

// NewProvider creates a provider based on config type
func NewProvider(ctx context.Context, config ProviderConfig) (Provider, error) {
	switch config.Type {
	case "github":
		return NewGitHubProvider(config)
	case "google":
		return NewGoogleProvider(ctx, config)
	case "oidc":
		return NewOIDCProvider(ctx, config)
	default:
		return nil, fmt.Errorf("unknown provider type: %s", config.Type)
	}
}

// GitHubProvider implements OAuth2 for GitHub
type GitHubProvider struct {
	name         string
	oauth2Config *oauth2.Config
}

// NewGitHubProvider creates a GitHub OAuth2 provider
func NewGitHubProvider(config ProviderConfig) (*GitHubProvider, error) {
	scopes := config.Scopes
	if scopes == nil {
		scopes = []string{"read:user", "user:email"}
	}

	oauth2Config := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Endpoint:     github.Endpoint,
		Scopes:       scopes,
	}

	return &GitHubProvider{
		name:         config.Name,
		oauth2Config: oauth2Config,
	}, nil
}

func (p *GitHubProvider) Name() string {
	return p.name
}

func (p *GitHubProvider) Issuer() string {
	return "https://github.com"
}

func (p *GitHubProvider) GetAuthURL(state string) string {
	return p.oauth2Config.AuthCodeURL(state)
}

func (p *GitHubProvider) ExchangeCode(ctx context.Context, code string) (*UserInfo, error) {
	token, err := p.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	client := p.oauth2Config.Client(ctx, token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return nil, fmt.Errorf("get user info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github API error: %s", string(body))
	}

	var githubUser struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&githubUser); err != nil {
		return nil, fmt.Errorf("decode user info: %w", err)
	}

	return &UserInfo{
		Subject:       fmt.Sprintf("%d", githubUser.ID),
		Email:         githubUser.Email,
		EmailVerified: githubUser.Email != "",
		Name:          githubUser.Name,
		Picture:       githubUser.AvatarURL,
	}, nil
}

// GoogleProvider implements OIDC for Google
type GoogleProvider struct {
	name         string
	oidcProvider *oidc.Provider
	oauth2Config *oauth2.Config
	verifier     *oidc.IDTokenVerifier
}

// NewGoogleProvider creates a Google OIDC provider
func NewGoogleProvider(ctx context.Context, config ProviderConfig) (*GoogleProvider, error) {
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, fmt.Errorf("create Google OIDC provider: %w", err)
	}

	scopes := config.Scopes
	if scopes == nil {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}

	oauth2Config := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Endpoint:     googleOAuth.Endpoint,
		Scopes:       scopes,
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: config.ClientID})

	return &GoogleProvider{
		name:         config.Name,
		oidcProvider: provider,
		oauth2Config: oauth2Config,
		verifier:     verifier,
	}, nil
}

func (p *GoogleProvider) Name() string {
	return p.name
}

func (p *GoogleProvider) Issuer() string {
	return "https://accounts.google.com"
}

func (p *GoogleProvider) GetAuthURL(state string) string {
	return p.oauth2Config.AuthCodeURL(state)
}

func (p *GoogleProvider) ExchangeCode(ctx context.Context, code string) (*UserInfo, error) {
	token, err := p.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in token response")
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verify ID token: %w", err)
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	return &UserInfo{
		Subject:       idToken.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		Name:          claims.Name,
		Picture:       claims.Picture,
	}, nil
}

// OIDCProvider implements generic OIDC
type OIDCProvider struct {
	name         string
	issuer       string
	oidcProvider *oidc.Provider
	oauth2Config *oauth2.Config
	verifier     *oidc.IDTokenVerifier
}

// NewOIDCProvider creates a generic OIDC provider
func NewOIDCProvider(ctx context.Context, config ProviderConfig) (*OIDCProvider, error) {
	if config.Issuer == "" {
		return nil, fmt.Errorf("issuer is required for generic OIDC provider")
	}

	provider, err := oidc.NewProvider(ctx, config.Issuer)
	if err != nil {
		return nil, fmt.Errorf("create OIDC provider: %w", err)
	}

	scopes := config.Scopes
	if scopes == nil {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}

	oauth2Config := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: config.ClientID})

	return &OIDCProvider{
		name:         config.Name,
		issuer:       config.Issuer,
		oidcProvider: provider,
		oauth2Config: oauth2Config,
		verifier:     verifier,
	}, nil
}

func (p *OIDCProvider) Name() string {
	return p.name
}

func (p *OIDCProvider) Issuer() string {
	return p.issuer
}

func (p *OIDCProvider) GetAuthURL(state string) string {
	return p.oauth2Config.AuthCodeURL(state)
}

func (p *OIDCProvider) ExchangeCode(ctx context.Context, code string) (*UserInfo, error) {
	token, err := p.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in token response")
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verify ID token: %w", err)
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	return &UserInfo{
		Subject:       idToken.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		Name:          claims.Name,
		Picture:       claims.Picture,
	}, nil
}

// AuthState represents the state for an auth flow
type AuthState struct {
	State        string
	Nonce        string
	RedirectURI  string
	ProviderName string
	CreatedAt    time.Time
}

// GenerateState generates a random state string
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Registry manages multiple auth providers
type Registry struct {
	providers      map[string]Provider
	authStateStore AuthStateStore
	mu             sync.RWMutex
	stateTTL       time.Duration
}

// NewRegistry creates a new provider registry with in-memory auth state store
func NewRegistry() *Registry {
	ttl := 10 * time.Minute
	return &Registry{
		providers:      make(map[string]Provider),
		authStateStore: NewMemoryAuthStateStore(ttl),
		stateTTL:       ttl,
	}
}

// NewRegistryWithStore creates a new provider registry with custom auth state store
func NewRegistryWithStore(store AuthStateStore) *Registry {
	return &Registry{
		providers:      make(map[string]Provider),
		authStateStore: store,
		stateTTL:       10 * time.Minute,
	}
}

// RegisterProvider registers a provider
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
func (r *Registry) GetProvider(name string) (Provider, bool) {
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
func (r *Registry) CreateAuthState(ctx context.Context, redirectURI, providerName string) (*AuthState, error) {
	state, err := GenerateState()
	if err != nil {
		return nil, err
	}

	nonce, err := GenerateState()
	if err != nil {
		return nil, err
	}

	authState := &AuthState{
		State:        state,
		Nonce:        nonce,
		RedirectURI:  redirectURI,
		ProviderName: providerName,
		CreatedAt:    time.Now(),
	}

	if err := r.authStateStore.Create(ctx, authState); err != nil {
		return nil, err
	}

	return authState, nil
}

// ValidateState validates and consumes an auth state
func (r *Registry) ValidateState(ctx context.Context, state string) (*AuthState, bool) {
	authState, err := r.authStateStore.Get(ctx, state)
	if err != nil || authState == nil {
		return nil, false
	}

	if time.Since(authState.CreatedAt) > r.stateTTL {
		_ = r.authStateStore.Delete(ctx, state)
		return nil, false
	}

	_ = r.authStateStore.Delete(ctx, state)
	return authState, true
}

// CleanupExpiredStates removes expired auth states
func (r *Registry) CleanupExpiredStates(ctx context.Context) {
	_ = r.authStateStore.DeleteExpired(ctx)
}
