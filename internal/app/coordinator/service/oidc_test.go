package service

import (
	"net/url"
	"testing"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/jwtauth"
)

func TestOIDCService_GenerateAuthURL(t *testing.T) {
	svc := NewOIDCService(OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "https://app.example.com/callback",
		StateTTL:     10 * time.Minute,
	}, nil)

	authURL, state, err := svc.GenerateAuthURL()
	if err != nil {
		t.Fatalf("GenerateAuthURL() error = %v", err)
	}

	if state == "" {
		t.Error("GenerateAuthURL() returned empty state")
	}

	parsedURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("Parse authURL error = %v", err)
	}

	expectedHost := "auth.example.com"
	if parsedURL.Host != expectedHost {
		t.Errorf("authURL host = %v, want %v", parsedURL.Host, expectedHost)
	}

	expectedPath := "/realms/test-realm/protocol/openid-connect/auth"
	if parsedURL.Path != expectedPath {
		t.Errorf("authURL path = %v, want %v", parsedURL.Path, expectedPath)
	}

	query := parsedURL.Query()
	if query.Get("client_id") != "test-client" {
		t.Errorf("client_id = %v, want %v", query.Get("client_id"), "test-client")
	}
	if query.Get("redirect_uri") != "https://app.example.com/callback" {
		t.Errorf("redirect_uri = %v, want %v", query.Get("redirect_uri"), "https://app.example.com/callback")
	}
	if query.Get("response_type") != "code" {
		t.Errorf("response_type = %v, want %v", query.Get("response_type"), "code")
	}
	if query.Get("scope") != "openid profile email" {
		t.Errorf("scope = %v, want %v", query.Get("scope"), "openid profile email")
	}
	if query.Get("state") != state {
		t.Errorf("state in URL = %v, want %v", query.Get("state"), state)
	}
}

func TestOIDCService_ValidateState(t *testing.T) {
	svc := NewOIDCService(OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "https://app.example.com/callback",
		StateTTL:     10 * time.Minute,
	}, nil)

	_, state, err := svc.GenerateAuthURL()
	if err != nil {
		t.Fatalf("GenerateAuthURL() error = %v", err)
	}

	if !svc.ValidateState(state) {
		t.Error("ValidateState() returned false for valid state")
	}

	if svc.ValidateState(state) {
		t.Error("ValidateState() returned true for already-used state (should be one-time use)")
	}
}

func TestOIDCService_ValidateState_Invalid(t *testing.T) {
	svc := NewOIDCService(OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "https://app.example.com/callback",
		StateTTL:     10 * time.Minute,
	}, nil)

	if svc.ValidateState("nonexistent-state") {
		t.Error("ValidateState() returned true for nonexistent state")
	}
}

func TestOIDCService_ValidateState_Expired(t *testing.T) {
	svc := NewOIDCService(OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "https://app.example.com/callback",
		StateTTL:     1 * time.Millisecond,
	}, nil)

	_, state, err := svc.GenerateAuthURL()
	if err != nil {
		t.Fatalf("GenerateAuthURL() error = %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if svc.ValidateState(state) {
		t.Error("ValidateState() returned true for expired state")
	}
}

func TestOIDCService_UniqueStates(t *testing.T) {
	svc := NewOIDCService(OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "https://app.example.com/callback",
		StateTTL:     10 * time.Minute,
	}, nil)

	states := make(map[string]bool)
	for i := 0; i < 100; i++ {
		_, state, err := svc.GenerateAuthURL()
		if err != nil {
			t.Fatalf("GenerateAuthURL() error = %v", err)
		}
		if states[state] {
			t.Errorf("GenerateAuthURL() generated duplicate state: %v", state)
		}
		states[state] = true
	}
}

func TestNewOIDCService_DefaultStateTTL(t *testing.T) {
	validator := jwtauth.NewValidator(jwtauth.ValidatorConfig{})
	svc := NewOIDCService(OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "https://app.example.com/callback",
	}, validator)

	if svc.config.StateTTL != 10*time.Minute {
		t.Errorf("default StateTTL = %v, want %v", svc.config.StateTTL, 10*time.Minute)
	}
}
