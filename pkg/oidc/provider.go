// Package oidc provides multi-provider OIDC and OAuth2 authentication support.
// It includes implementations for GitHub OAuth2, Google OIDC, and generic OIDC providers,
// along with a registry for managing multiple providers and their authentication states.
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

// UserInfo represents normalized user information retrieved from any auth provider.
// It provides a common structure for user data regardless of the underlying
// authentication mechanism (OAuth2 or OIDC).
type UserInfo struct {
	Subject       string `json:"sub"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified,omitempty"`
	Name          string `json:"name,omitempty"`
	Picture       string `json:"picture,omitempty"`
}

// Provider defines the interface that all authentication providers must implement.
// It abstracts the differences between OAuth2 and OIDC providers, allowing the
// coordinator to handle authentication uniformly.
type Provider interface {
	// Name returns the unique identifier for this provider (e.g., "github", "google").
	Name() string

	// Issuer returns the provider's issuer URL used as the authority identifier.
	Issuer() string

	// GetAuthURL generates the authorization URL that users should be redirected to.
	// The state parameter is used for CSRF protection.
	GetAuthURL(state string) string

	// ExchangeCode exchanges an authorization code for user information.
	// It handles token exchange and user info retrieval internally.
	ExchangeCode(ctx context.Context, code string) (*UserInfo, error)
}

// ProviderConfig contains the configuration needed to create an authentication provider.
// It supports GitHub OAuth2, Google OIDC, and generic OIDC providers.
type ProviderConfig struct {
	// Type specifies the provider type: "github", "google", or "oidc".
	Type string `json:"type"`

	// Name is a unique identifier for this provider instance.
	Name string `json:"name"`

	// Issuer is the OIDC issuer URL. Required for "oidc" type, ignored for others.
	Issuer string `json:"issuer,omitempty"`

	// ClientID is the OAuth2/OIDC client ID.
	ClientID string `json:"clientId"`

	// ClientSecret is the OAuth2/OIDC client secret.
	ClientSecret string `json:"clientSecret"`

	// Scopes specifies the OAuth2 scopes to request. If nil, provider-specific
	// defaults are used (e.g., "read:user,user:email" for GitHub).
	Scopes []string `json:"scopes,omitempty"`

	// RedirectURL is the callback URL for the OAuth2/OIDC flow.
	RedirectURL string `json:"redirectUrl"`
}

// NewProvider creates a new Provider based on the config type.
// It returns an error if the provider type is unknown or if provider
// initialization fails (e.g., cannot reach OIDC discovery endpoint).
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

// GitHubProvider implements the Provider interface using GitHub's OAuth2 API.
// It retrieves user information from GitHub's /user endpoint after token exchange.
type GitHubProvider struct {
	name         string
	oauth2Config *oauth2.Config
}

// NewGitHubProvider creates a new GitHubProvider with the given configuration.
// If Scopes is nil in the config, it defaults to ["read:user", "user:email"].
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

// Name returns the provider's configured name.
func (p *GitHubProvider) Name() string {
	return p.name
}

// Issuer returns GitHub's base URL as the issuer identifier.
func (p *GitHubProvider) Issuer() string {
	return "https://github.com"
}

// GetAuthURL generates the GitHub OAuth2 authorization URL with the given state.
func (p *GitHubProvider) GetAuthURL(state string) string {
	return p.oauth2Config.AuthCodeURL(state)
}

// ExchangeCode exchanges an authorization code for GitHub user information.
// It calls GitHub's /user API endpoint to retrieve the user's profile data.
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

// GoogleProvider implements the Provider interface using Google's OIDC.
// It uses ID token verification for secure user identity claims.
type GoogleProvider struct {
	name         string
	oidcProvider *oidc.Provider
	oauth2Config *oauth2.Config
	verifier     *oidc.IDTokenVerifier
}

// NewGoogleProvider creates a new GoogleProvider using OIDC discovery.
// It fetches the provider configuration from Google's well-known endpoint.
// If Scopes is nil in the config, it defaults to ["openid", "profile", "email"].
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

// Name returns the provider's configured name.
func (p *GoogleProvider) Name() string {
	return p.name
}

// Issuer returns Google's OIDC issuer URL.
func (p *GoogleProvider) Issuer() string {
	return "https://accounts.google.com"
}

// GetAuthURL generates the Google OAuth2 authorization URL with the given state.
func (p *GoogleProvider) GetAuthURL(state string) string {
	return p.oauth2Config.AuthCodeURL(state)
}

// ExchangeCode exchanges an authorization code for Google user information.
// It verifies the ID token signature and extracts user claims.
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

// OIDCProvider implements the Provider interface for any OIDC-compliant identity provider.
// It uses standard OIDC discovery and ID token verification.
type OIDCProvider struct {
	name         string
	issuer       string
	oidcProvider *oidc.Provider
	oauth2Config *oauth2.Config
	verifier     *oidc.IDTokenVerifier
}

// NewOIDCProvider creates a new generic OIDC provider using discovery.
// The Issuer field in config is required and must point to a valid OIDC issuer URL.
// If Scopes is nil in the config, it defaults to ["openid", "profile", "email"].
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

// Name returns the provider's configured name.
func (p *OIDCProvider) Name() string {
	return p.name
}

// Issuer returns the configured OIDC issuer URL.
func (p *OIDCProvider) Issuer() string {
	return p.issuer
}

// GetAuthURL generates the OIDC authorization URL with the given state.
func (p *OIDCProvider) GetAuthURL(state string) string {
	return p.oauth2Config.AuthCodeURL(state)
}

// ExchangeCode exchanges an authorization code for user information.
// It verifies the ID token signature and extracts standard OIDC claims.
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

// AuthState holds the temporary state for an in-progress OAuth2/OIDC authentication flow.
// It is used to correlate the callback request with the original authorization request
// and to prevent CSRF attacks.
type AuthState struct {
	// State is the random string used for CSRF protection.
	State string

	// Nonce is an additional random value for replay protection (used with OIDC).
	Nonce string

	// RedirectURI is where to redirect the user after successful authentication.
	RedirectURI string

	// ProviderName identifies which provider initiated this auth flow.
	ProviderName string

	// CreatedAt records when this state was created for TTL enforcement.
	CreatedAt time.Time
}

// GenerateState generates a cryptographically random, URL-safe base64-encoded string.
// It is used for OAuth2 state parameters and OIDC nonces.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Registry manages multiple authentication providers and their associated auth states.
// It is safe for concurrent use and handles provider registration, lookup, and
// authentication state lifecycle management.
type Registry struct {
	providers      map[string]Provider
	authStateStore AuthStateStore
	mu             sync.RWMutex
	stateTTL       time.Duration
}

// NewRegistry creates a new Registry with an in-memory auth state store.
// Auth states expire after 10 minutes by default.
func NewRegistry() *Registry {
	ttl := 10 * time.Minute
	return &Registry{
		providers:      make(map[string]Provider),
		authStateStore: NewMemoryAuthStateStore(ttl),
		stateTTL:       ttl,
	}
}

// NewRegistryWithStore creates a new Registry with a custom auth state store.
// This allows using persistent storage (e.g., database-backed) for auth states
// in multi-instance deployments.
func NewRegistryWithStore(store AuthStateStore) *Registry {
	return &Registry{
		providers:      make(map[string]Provider),
		authStateStore: store,
		stateTTL:       10 * time.Minute,
	}
}

// RegisterProvider creates and registers a new provider with the given configuration.
// If a provider with the same name already exists, it will be replaced.
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

// GetProvider retrieves a registered provider by its name.
// Returns the provider and true if found, or nil and false if not found.
func (r *Registry) GetProvider(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, ok := r.providers[name]
	return provider, ok
}

// ListProviders returns the names of all registered providers.
// The order of names is not guaranteed.
func (r *Registry) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// CreateAuthState generates and stores a new auth state for an authentication flow.
// The returned AuthState contains the state parameter to include in the authorization URL.
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

// ValidateState validates and consumes an auth state from the callback.
// It returns the AuthState and true if valid, or nil and false if the state
// is not found, expired, or already consumed. The state is deleted after validation
// to prevent replay attacks.
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

// CleanupExpiredStates removes all expired auth states from the store.
// This should be called periodically to prevent memory leaks in the state store.
func (r *Registry) CleanupExpiredStates(ctx context.Context) {
	_ = r.authStateStore.DeleteExpired(ctx)
}
