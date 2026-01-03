package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

func TestOIDCController_HandleLogin(t *testing.T) {
	config := service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "wonder-mesh",
		ClientID:     "coordinator",
		ClientSecret: "secret",
		RedirectURI:  "https://coordinator.example.com/coordinator/oidc/callback",
	}
	oidcService := service.NewOIDCService(config, nil)
	controller := NewOIDCController(oidcService, nil, "https://coordinator.example.com", true)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/login", nil)
	rec := httptest.NewRecorder()

	controller.HandleLogin(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusFound)
	}

	location := rec.Header().Get("Location")
	if location == "" {
		t.Error("Location header should not be empty")
	}

	if !strings.HasPrefix(location, "https://auth.example.com/realms/wonder-mesh/protocol/openid-connect/auth") {
		t.Errorf("Location = %q, should start with Keycloak auth URL", location)
	}

	if !strings.Contains(location, "client_id=coordinator") {
		t.Errorf("Location = %q, should contain client_id", location)
	}

	if !strings.Contains(location, "response_type=code") {
		t.Errorf("Location = %q, should contain response_type=code", location)
	}

	if !strings.Contains(location, "state=") {
		t.Errorf("Location = %q, should contain state", location)
	}
}

func TestOIDCController_HandleCallback_MissingCode(t *testing.T) {
	config := service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "wonder-mesh",
		ClientID:     "coordinator",
		ClientSecret: "secret",
		RedirectURI:  "https://coordinator.example.com/coordinator/oidc/callback",
	}
	oidcService := service.NewOIDCService(config, nil)
	controller := NewOIDCController(oidcService, nil, "https://coordinator.example.com", true)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/callback?state=valid-state", nil)
	rec := httptest.NewRecorder()

	controller.HandleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestOIDCController_HandleCallback_MissingState(t *testing.T) {
	config := service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "wonder-mesh",
		ClientID:     "coordinator",
		ClientSecret: "secret",
		RedirectURI:  "https://coordinator.example.com/coordinator/oidc/callback",
	}
	oidcService := service.NewOIDCService(config, nil)
	controller := NewOIDCController(oidcService, nil, "https://coordinator.example.com", true)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/callback?code=auth-code", nil)
	rec := httptest.NewRecorder()

	controller.HandleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestOIDCController_HandleCallback_InvalidState(t *testing.T) {
	config := service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "wonder-mesh",
		ClientID:     "coordinator",
		ClientSecret: "secret",
		RedirectURI:  "https://coordinator.example.com/coordinator/oidc/callback",
	}
	oidcService := service.NewOIDCService(config, nil)
	controller := NewOIDCController(oidcService, nil, "https://coordinator.example.com", true)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/callback?code=auth-code&state=invalid-state", nil)
	rec := httptest.NewRecorder()

	controller.HandleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestOIDCController_HandleCallback_OAuthError(t *testing.T) {
	config := service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "wonder-mesh",
		ClientID:     "coordinator",
		ClientSecret: "secret",
		RedirectURI:  "https://coordinator.example.com/coordinator/oidc/callback",
	}
	oidcService := service.NewOIDCService(config, nil)
	controller := NewOIDCController(oidcService, nil, "https://coordinator.example.com", true)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/callback?error=access_denied&error_description=User+denied+access", nil)
	rec := httptest.NewRecorder()

	controller.HandleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "User denied access") {
		t.Errorf("body = %q, should contain error description", body)
	}
}

func TestOIDCController_HandleLogout(t *testing.T) {
	config := service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "wonder-mesh",
		ClientID:     "coordinator",
		ClientSecret: "secret",
		RedirectURI:  "https://coordinator.example.com/coordinator/oidc/callback",
	}
	oidcService := service.NewOIDCService(config, nil)
	controller := NewOIDCController(oidcService, nil, "https://coordinator.example.com", true)

	sessionID, _ := oidcService.CreateSession("user-123", "access-token", "refresh-token", 3600)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/logout", nil)
	req.AddCookie(&http.Cookie{Name: oidcService.GetSessionCookieName(), Value: sessionID})
	rec := httptest.NewRecorder()

	controller.HandleLogout(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusFound)
	}

	location := rec.Header().Get("Location")
	if location != "/" {
		t.Errorf("Location = %q, want %q", location, "/")
	}

	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == oidcService.GetSessionCookieName() {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Error("session cookie should be in response")
	} else if sessionCookie.MaxAge != -1 {
		t.Errorf("session cookie MaxAge = %d, want -1 (expire)", sessionCookie.MaxAge)
	}

	if _, err := oidcService.GetSession(sessionID); err != service.ErrSessionNotFound {
		t.Errorf("GetSession after logout = %v, want ErrSessionNotFound", err)
	}
}

func TestOIDCController_DeterminePostLoginRedirect(t *testing.T) {
	config := service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "wonder-mesh",
		ClientID:     "coordinator",
		ClientSecret: "secret",
		RedirectURI:  "https://coordinator.example.com/coordinator/oidc/callback",
	}
	oidcService := service.NewOIDCService(config, nil)
	controller := NewOIDCController(oidcService, nil, "https://coordinator.example.com", true)

	tests := []struct {
		name     string
		query    string
		want     string
	}{
		{"no redirect_to", "", "/"},
		{"valid path", "?redirect_to=/dashboard", "/dashboard"},
		{"same host", "?redirect_to=https://coordinator.example.com/settings", "/"},
		{"different host", "?redirect_to=https://evil.com/phish", "/"},
		{"invalid url", "?redirect_to=://invalid", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/callback"+tt.query, nil)
			got := controller.determinePostLoginRedirect(req)
			if got != tt.want {
				t.Errorf("determinePostLoginRedirect() = %q, want %q", got, tt.want)
			}
		})
	}
}
