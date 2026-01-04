package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

func TestOIDCController_HandleLogin(t *testing.T) {
	oidcService := service.NewOIDCService(service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "https://app.example.com/coordinator/oidc/callback",
		StateTTL:     10 * time.Minute,
	}, nil)

	controller := NewOIDCController(oidcService, nil, true)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/login", nil)
	rec := httptest.NewRecorder()

	controller.HandleLogin(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("HandleLogin() status = %v, want %v", resp.StatusCode, http.StatusFound)
	}

	location := resp.Header.Get("Location")
	if !strings.HasPrefix(location, "https://auth.example.com/realms/test-realm/protocol/openid-connect/auth") {
		t.Errorf("HandleLogin() redirect Location = %v, want prefix %v", location, "https://auth.example.com/realms/test-realm/protocol/openid-connect/auth")
	}

	cookies := resp.Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "wonder_oauth_state" {
			stateCookie = c
			break
		}
	}

	if stateCookie == nil {
		t.Fatal("HandleLogin() did not set wonder_oauth_state cookie")
	}

	if stateCookie.Value == "" {
		t.Error("HandleLogin() set empty state cookie")
	}

	if !stateCookie.HttpOnly {
		t.Error("HandleLogin() state cookie should be HttpOnly")
	}

	if !stateCookie.Secure {
		t.Error("HandleLogin() state cookie should be Secure when secureCookies=true")
	}

	if !strings.Contains(location, "state="+stateCookie.Value) {
		t.Errorf("HandleLogin() state in URL does not match cookie. URL: %v, Cookie: %v", location, stateCookie.Value)
	}
}

func TestOIDCController_HandleLogin_InsecureCookies(t *testing.T) {
	oidcService := service.NewOIDCService(service.OIDCConfig{
		KeycloakURL:  "http://localhost:8080",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "http://localhost:9080/coordinator/oidc/callback",
		StateTTL:     10 * time.Minute,
	}, nil)

	controller := NewOIDCController(oidcService, nil, false)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/login", nil)
	rec := httptest.NewRecorder()

	controller.HandleLogin(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	var stateCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "wonder_oauth_state" {
			stateCookie = c
			break
		}
	}

	if stateCookie == nil {
		t.Fatal("HandleLogin() did not set wonder_oauth_state cookie")
	}

	if stateCookie.Secure {
		t.Error("HandleLogin() state cookie should not be Secure when secureCookies=false")
	}
}

func TestOIDCController_HandleCallback_MissingCode(t *testing.T) {
	oidcService := service.NewOIDCService(service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "https://app.example.com/coordinator/oidc/callback",
		StateTTL:     10 * time.Minute,
	}, nil)

	controller := NewOIDCController(oidcService, nil, true)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/callback?state=abc", nil)
	rec := httptest.NewRecorder()

	controller.HandleCallback(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("HandleCallback() status = %v, want %v", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestOIDCController_HandleCallback_MissingState(t *testing.T) {
	oidcService := service.NewOIDCService(service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "https://app.example.com/coordinator/oidc/callback",
		StateTTL:     10 * time.Minute,
	}, nil)

	controller := NewOIDCController(oidcService, nil, true)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/callback?code=abc", nil)
	rec := httptest.NewRecorder()

	controller.HandleCallback(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("HandleCallback() status = %v, want %v", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestOIDCController_HandleCallback_StateMismatch(t *testing.T) {
	oidcService := service.NewOIDCService(service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "https://app.example.com/coordinator/oidc/callback",
		StateTTL:     10 * time.Minute,
	}, nil)

	controller := NewOIDCController(oidcService, nil, true)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/callback?code=abc&state=xyz", nil)
	req.AddCookie(&http.Cookie{
		Name:  "wonder_oauth_state",
		Value: "different-state",
	})
	rec := httptest.NewRecorder()

	controller.HandleCallback(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("HandleCallback() status = %v, want %v", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestOIDCController_HandleCallback_OAuthError(t *testing.T) {
	oidcService := service.NewOIDCService(service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "https://app.example.com/coordinator/oidc/callback",
		StateTTL:     10 * time.Minute,
	}, nil)

	controller := NewOIDCController(oidcService, nil, true)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/callback?error=access_denied&error_description=User+denied+access", nil)
	rec := httptest.NewRecorder()

	controller.HandleCallback(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("HandleCallback() status = %v, want %v", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestOIDCController_HandleCallback_InvalidState(t *testing.T) {
	oidcService := service.NewOIDCService(service.OIDCConfig{
		KeycloakURL:  "https://auth.example.com",
		Realm:        "test-realm",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURI:  "https://app.example.com/coordinator/oidc/callback",
		StateTTL:     10 * time.Minute,
	}, nil)

	controller := NewOIDCController(oidcService, nil, true)

	req := httptest.NewRequest(http.MethodGet, "/coordinator/oidc/callback?code=abc&state=nonexistent", nil)
	req.AddCookie(&http.Cookie{
		Name:  "wonder_oauth_state",
		Value: "nonexistent",
	})
	rec := httptest.NewRecorder()

	controller.HandleCallback(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("HandleCallback() status = %v, want %v", resp.StatusCode, http.StatusBadRequest)
	}
}
