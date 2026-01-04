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

	sessionID, _, _ := oidcService.CreateSession("user-123", "access-token", "refresh-token", 3600)

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
		name  string
		query string
		want  string
	}{
		{"no redirect_to", "", "/"},
		{"valid path", "?redirect_to=/dashboard", "/dashboard"},
		{"valid nested path", "?redirect_to=/api/v1/nodes", "/api/v1/nodes"},
		{"valid path with query", "?redirect_to=/dashboard?tab=settings", "/dashboard?tab=settings"},
		{"absolute URL same host", "?redirect_to=https://coordinator.example.com/settings", "/"},
		{"absolute URL different host", "?redirect_to=https://evil.com/phish", "/"},
		{"protocol-relative URL", "?redirect_to=//evil.com/phish", "/"},
		{"javascript scheme", "?redirect_to=javascript:alert(1)", "/"},
		{"data scheme", "?redirect_to=data:text/html,<script>alert(1)</script>", "/"},
		{"empty path", "?redirect_to=", "/"},
		{"just slash", "?redirect_to=/", "/"},
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

func TestIsSafeRedirectPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/", true},
		{"/dashboard", true},
		{"/api/v1/nodes", true},
		{"/path?query=value", true},
		{"", false},
		{"//evil.com", false},
		{"//evil.com/path", false},
		{"javascript:alert(1)", false},
		{"data:text/html,<script>", false},
		{"https://evil.com", false},
		{"http://evil.com", false},
		{"ftp://evil.com", false},
		{"relative/path", false},
		{"../parent", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isSafeRedirectPath(tt.path)
			if got != tt.want {
				t.Errorf("isSafeRedirectPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
