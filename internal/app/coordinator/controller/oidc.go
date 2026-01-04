package controller

import (
	"log/slog"
	"net/http"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

const (
	sessionCookieName = "wonder_session"
	stateCookieName   = "wonder_oauth_state"
)

// OIDCController handles OIDC login and callback endpoints.
type OIDCController struct {
	oidcService     *service.OIDCService
	wonderNetService *service.WonderNetService
	secureCookies   bool
}

// NewOIDCController creates a new OIDCController.
func NewOIDCController(
	oidcService *service.OIDCService,
	wonderNetService *service.WonderNetService,
	secureCookies bool,
) *OIDCController {
	return &OIDCController{
		oidcService:     oidcService,
		wonderNetService: wonderNetService,
		secureCookies:   secureCookies,
	}
}

// HandleLogin handles GET /coordinator/oidc/login.
// It initiates the OIDC authorization flow by redirecting to Keycloak.
func (c *OIDCController) HandleLogin(w http.ResponseWriter, r *http.Request) {
	authURL, state, err := c.oidcService.GenerateAuthURL()
	if err != nil {
		slog.Error("generate auth URL", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/coordinator/oidc",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   c.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback handles GET /coordinator/oidc/callback.
// It completes the OIDC authorization flow by exchanging the code for tokens.
func (c *OIDCController) HandleCallback(w http.ResponseWriter, r *http.Request) {
	if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
		errDesc := r.URL.Query().Get("error_description")
		slog.Warn("OAuth error from provider", "error", oauthErr, "description", errDesc)
		http.Error(w, "authentication failed: "+oauthErr, http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "missing state parameter", http.StatusBadRequest)
		return
	}

	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil || stateCookie.Value != state {
		http.Error(w, "invalid state parameter", http.StatusBadRequest)
		return
	}

	if !c.oidcService.ValidateState(state) {
		http.Error(w, "state expired or already used", http.StatusBadRequest)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/coordinator/oidc",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	tokens, err := c.oidcService.ExchangeCode(r.Context(), code)
	if err != nil {
		slog.Error("exchange authorization code", "error", err)
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	claims, err := c.oidcService.ValidateIDToken(tokens.IDToken)
	if err != nil {
		slog.Error("validate ID token", "error", err)
		http.Error(w, "invalid ID token", http.StatusInternalServerError)
		return
	}

	_, err = c.wonderNetService.ResolveWonderNetFromClaims(r.Context(), claims)
	if err != nil {
		slog.Error("resolve wonder net from claims", "error", err)
		http.Error(w, "provision user failed", http.StatusInternalServerError)
		return
	}

	maxAge := tokens.ExpiresIn
	if maxAge <= 0 {
		maxAge = 3600
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    tokens.IDToken,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   c.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
  <title>Login Successful</title>
  <meta http-equiv="refresh" content="2;url=/coordinator/api/v1/nodes">
</head>
<body>
  <h1>Login Successful</h1>
  <p>You will be redirected shortly. If not, <a href="/coordinator/api/v1/nodes">click here</a>.</p>
</body>
</html>`))
}
