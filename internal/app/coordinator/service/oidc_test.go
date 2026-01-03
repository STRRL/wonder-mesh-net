package service

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestOIDCService_GenerateAuthURL(t *testing.T) {
	config := OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "wonder-mesh",
		ClientID:     "coordinator",
		ClientSecret: "secret",
		RedirectURI:  "https://coordinator.example.com/coordinator/oidc/callback",
	}
	svc := NewOIDCService(config, nil)

	authURL, state, err := svc.GenerateAuthURL()
	if err != nil {
		t.Fatalf("GenerateAuthURL: %v", err)
	}

	if state == "" {
		t.Error("state should not be empty")
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}

	expectedPath := "/realms/wonder-mesh/protocol/openid-connect/auth"
	if parsed.Path != expectedPath {
		t.Errorf("path = %q, want %q", parsed.Path, expectedPath)
	}

	query := parsed.Query()
	if got := query.Get("client_id"); got != "coordinator" {
		t.Errorf("client_id = %q, want %q", got, "coordinator")
	}
	if got := query.Get("response_type"); got != "code" {
		t.Errorf("response_type = %q, want %q", got, "code")
	}
	if got := query.Get("scope"); !strings.Contains(got, "openid") {
		t.Errorf("scope = %q, should contain 'openid'", got)
	}
	if got := query.Get("redirect_uri"); got != config.RedirectURI {
		t.Errorf("redirect_uri = %q, want %q", got, config.RedirectURI)
	}
	if got := query.Get("state"); got != state {
		t.Errorf("state = %q, want %q", got, state)
	}
}

func TestOIDCService_ValidateState(t *testing.T) {
	config := OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "wonder-mesh",
		ClientID:     "coordinator",
		ClientSecret: "secret",
		RedirectURI:  "https://coordinator.example.com/coordinator/oidc/callback",
	}
	svc := NewOIDCService(config, nil)

	_, validState, err := svc.GenerateAuthURL()
	if err != nil {
		t.Fatalf("GenerateAuthURL: %v", err)
	}

	if err := svc.ValidateState(validState); err != nil {
		t.Errorf("ValidateState(validState): %v", err)
	}

	if err := svc.ValidateState(validState); err != ErrInvalidState {
		t.Errorf("ValidateState(validState) second time = %v, want ErrInvalidState", err)
	}

	if err := svc.ValidateState("invalid-state"); err != ErrInvalidState {
		t.Errorf("ValidateState(invalid-state) = %v, want ErrInvalidState", err)
	}
}

func TestOIDCService_Session(t *testing.T) {
	config := OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "wonder-mesh",
		ClientID:     "coordinator",
		ClientSecret: "secret",
		RedirectURI:  "https://coordinator.example.com/coordinator/oidc/callback",
	}
	svc := NewOIDCService(config, nil)

	sessionID, err := svc.CreateSession("user-123", "access-token", "refresh-token", 3600)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sessionID == "" {
		t.Error("sessionID should not be empty")
	}

	session, err := svc.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.UserID != "user-123" {
		t.Errorf("session.UserID = %q, want %q", session.UserID, "user-123")
	}
	if session.AccessToken != "access-token" {
		t.Errorf("session.AccessToken = %q, want %q", session.AccessToken, "access-token")
	}
	if session.RefreshToken != "refresh-token" {
		t.Errorf("session.RefreshToken = %q, want %q", session.RefreshToken, "refresh-token")
	}

	if _, err := svc.GetSession("invalid-session"); err != ErrSessionNotFound {
		t.Errorf("GetSession(invalid) = %v, want ErrSessionNotFound", err)
	}

	svc.DeleteSession(sessionID)
	if _, err := svc.GetSession(sessionID); err != ErrSessionNotFound {
		t.Errorf("GetSession after delete = %v, want ErrSessionNotFound", err)
	}
}

func TestOIDCService_CleanupExpiredStates(t *testing.T) {
	config := OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "wonder-mesh",
		ClientID:     "coordinator",
		ClientSecret: "secret",
		RedirectURI:  "https://coordinator.example.com/coordinator/oidc/callback",
	}
	svc := NewOIDCService(config, nil)

	_, state, err := svc.GenerateAuthURL()
	if err != nil {
		t.Fatalf("GenerateAuthURL: %v", err)
	}

	svc.stateMu.Lock()
	svc.states[state] = time.Now().Add(-1 * time.Hour)
	svc.stateMu.Unlock()

	svc.CleanupExpiredStates()

	if err := svc.ValidateState(state); err != ErrInvalidState {
		t.Errorf("ValidateState after cleanup = %v, want ErrInvalidState", err)
	}
}

func TestGenerateRandomString(t *testing.T) {
	s1, err := generateRandomString(32)
	if err != nil {
		t.Fatalf("generateRandomString: %v", err)
	}
	if len(s1) != 32 {
		t.Errorf("len(s1) = %d, want 32", len(s1))
	}

	s2, err := generateRandomString(32)
	if err != nil {
		t.Fatalf("generateRandomString: %v", err)
	}
	if s1 == s2 {
		t.Error("two random strings should be different")
	}
}

func TestHashSessionID(t *testing.T) {
	hash1 := hashSessionID("session-123")
	hash2 := hashSessionID("session-123")
	if hash1 != hash2 {
		t.Error("same input should produce same hash")
	}

	hash3 := hashSessionID("session-456")
	if hash1 == hash3 {
		t.Error("different inputs should produce different hashes")
	}

	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64 (SHA256 hex)", len(hash1))
	}
}
