package jwtauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

var (
	ErrInvalidToken    = errors.New("invalid token")
	ErrExpiredToken    = errors.New("token expired")
	ErrMissingClaim    = errors.New("missing required claim")
	ErrInvalidAudience = errors.New("invalid audience")
	ErrInvalidIssuer   = errors.New("invalid issuer")
	ErrJWKSFetchFailed = errors.New("JWKS fetch failed")
)

// Claims represents the expected JWT claims from Keycloak.
type Claims struct {
	jwt.RegisteredClaims

	// Keycloak standard claims
	PreferredUsername string `json:"preferred_username,omitempty"`
	Email             string `json:"email,omitempty"`
	EmailVerified     bool   `json:"email_verified,omitempty"`
	Name              string `json:"name,omitempty"`
	GivenName         string `json:"given_name,omitempty"`
	FamilyName        string `json:"family_name,omitempty"`

	// Keycloak realm roles
	RealmAccess struct {
		Roles []string `json:"roles,omitempty"`
	} `json:"realm_access,omitempty"`

	// Client-specific roles
	ResourceAccess map[string]struct {
		Roles []string `json:"roles,omitempty"`
	} `json:"resource_access,omitempty"`

	// Azp is the authorized party (client ID that requested the token)
	Azp string `json:"azp,omitempty"`

	// Scope is the OAuth scope
	Scope string `json:"scope,omitempty"`
}

// ValidatorConfig holds configuration for the JWT validator.
type ValidatorConfig struct {
	JWKSURL         string
	Issuer          string
	Audience        string
	RefreshInterval time.Duration
}

// Validator validates JWTs using JWKS from Keycloak.
type Validator struct {
	config  ValidatorConfig
	keySet  jwk.Set
	mu      sync.RWMutex
	lastErr error
}

// NewValidator creates a new JWT validator.
func NewValidator(config ValidatorConfig) *Validator {
	if config.RefreshInterval == 0 {
		config.RefreshInterval = 5 * time.Minute
	}

	return &Validator{
		config: config,
	}
}

// Start begins the background JWKS refresh goroutine.
func (v *Validator) Start(ctx context.Context) error {
	if err := v.refreshJWKS(ctx); err != nil {
		return fmt.Errorf("initial JWKS fetch: %w", err)
	}

	go v.refreshLoop(ctx)
	return nil
}

func (v *Validator) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(v.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := v.refreshJWKS(ctx); err != nil {
				v.mu.Lock()
				v.lastErr = err
				v.mu.Unlock()
			}
		}
	}
}

func (v *Validator) refreshJWKS(ctx context.Context) error {
	keySet, err := jwk.Fetch(ctx, v.config.JWKSURL)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}

	v.mu.Lock()
	v.keySet = keySet
	v.lastErr = nil
	v.mu.Unlock()

	return nil
}

// Validate validates a JWT token and returns the claims.
func (v *Validator) Validate(tokenString string) (*Claims, error) {
	v.mu.RLock()
	keySet := v.keySet
	v.mu.RUnlock()

	if keySet == nil {
		return nil, ErrJWKSFetchFailed
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("missing kid in token header")
		}

		key, ok := keySet.LookupKeyID(kid)
		if !ok {
			return nil, fmt.Errorf("key %s not found in JWKS", kid)
		}

		var rawKey interface{}
		if err := key.Raw(&rawKey); err != nil {
			return nil, fmt.Errorf("extract raw key: %w", err)
		}

		return rawKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %s", ErrInvalidToken, err.Error())
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	if v.config.Issuer != "" && claims.Issuer != v.config.Issuer {
		return nil, fmt.Errorf("%w: got %s, want %s", ErrInvalidIssuer, claims.Issuer, v.config.Issuer)
	}

	if v.config.Audience != "" {
		// Service accounts created by coordinator have azp matching their own client ID
		// (e.g., wonder-net-xxx-deployer), not the main client. Since these are created
		// via Keycloak Admin API by coordinator itself, they are trusted.
		isServiceAccount := strings.HasPrefix(claims.PreferredUsername, "service-account-")

		if !isServiceAccount {
			found := false
			for _, aud := range claims.Audience {
				if aud == v.config.Audience {
					found = true
					break
				}
			}
			// Keycloak doesn't include aud claim by default, but always includes azp (authorized party)
			// which contains the client ID that requested the token
			if !found && claims.Azp == v.config.Audience {
				found = true
			}
			if !found {
				return nil, fmt.Errorf("%w: %s not in aud=%v azp=%s", ErrInvalidAudience, v.config.Audience, claims.Audience, claims.Azp)
			}
		}
	}

	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: sub", ErrMissingClaim)
	}

	return claims, nil
}

// HasRole checks if the claims contain a specific realm role.
func (c *Claims) HasRole(role string) bool {
	for _, r := range c.RealmAccess.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasClientRole checks if the claims contain a specific client role.
func (c *Claims) HasClientRole(clientID, role string) bool {
	if access, ok := c.ResourceAccess[clientID]; ok {
		for _, r := range access.Roles {
			if r == role {
				return true
			}
		}
	}
	return false
}

// IsServiceAccount returns true if this token was issued for a service account.
// Keycloak service accounts have preferred_username starting with "service-account-".
func (c *Claims) IsServiceAccount() bool {
	return strings.HasPrefix(c.PreferredUsername, "service-account-")
}
