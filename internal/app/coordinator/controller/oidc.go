package controller

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// isSafeRedirectPath checks if the redirect path is safe to use.
// Only allows relative paths starting with "/" but not "//".
// This prevents open redirect attacks via javascript:, data:, or protocol-relative URLs.
func isSafeRedirectPath(path string) bool {
	if path == "" {
		return false
	}
	if !strings.HasPrefix(path, "/") {
		return false
	}
	if strings.HasPrefix(path, "//") {
		return false
	}
	return true
}

const (
	defaultPostLoginRedirect = "/ui/"
)

// OIDCController handles OIDC authentication endpoints.
type OIDCController struct {
	oidcService      *service.OIDCService
	wonderNetService *service.WonderNetService
	publicURL        string
	secureCookie     bool
}

// NewOIDCController creates a new OIDC controller.
func NewOIDCController(
	oidcService *service.OIDCService,
	wonderNetService *service.WonderNetService,
	publicURL string,
	secureCookie bool,
) *OIDCController {
	return &OIDCController{
		oidcService:      oidcService,
		wonderNetService: wonderNetService,
		publicURL:        publicURL,
		secureCookie:     secureCookie,
	}
}

// HandleLogin initiates the OIDC login flow.
// GET /coordinator/oidc/login
func (c *OIDCController) HandleLogin(w http.ResponseWriter, r *http.Request) {
	authURL, state, err := c.oidcService.GenerateAuthURL()
	if err != nil {
		slog.Error("generate auth URL", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	slog.Debug("OIDC login initiated", "state", state[:8]+"...")

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback handles the OIDC callback after user authentication.
// GET /coordinator/oidc/callback?code=xxx&state=xxx
func (c *OIDCController) HandleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")
	errorDesc := r.URL.Query().Get("error_description")

	if errorParam != "" {
		slog.Warn("OIDC callback error", "error", errorParam, "description", errorDesc)
		http.Error(w, "authentication failed: "+errorDesc, http.StatusBadRequest)
		return
	}

	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	if state == "" {
		http.Error(w, "missing state parameter", http.StatusBadRequest)
		return
	}

	if err := c.oidcService.ValidateState(state); err != nil {
		slog.Warn("OIDC state validation failed", "error", err)
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	tokenResp, err := c.oidcService.ExchangeCode(r.Context(), code)
	if err != nil {
		slog.Error("OIDC token exchange", "error", err)
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	claims, err := c.oidcService.ValidateIDToken(tokenResp.IDToken)
	if err != nil {
		slog.Error("OIDC ID token validation", "error", err)
		http.Error(w, "invalid ID token", http.StatusInternalServerError)
		return
	}

	slog.Info("OIDC login successful",
		"sub", claims.Subject,
		"username", claims.PreferredUsername,
		"email", claims.Email,
	)

	_, err = c.wonderNetService.ResolveWonderNetFromClaims(r.Context(), claims)
	if err != nil {
		slog.Error("resolve wonder net", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	sessionID, sessionTTL, err := c.oidcService.CreateSession(
		claims.Subject,
		tokenResp.AccessToken,
		tokenResp.RefreshToken,
		tokenResp.ExpiresIn,
	)
	if err != nil {
		slog.Error("create session", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	cookie := &http.Cookie{
		Name:     c.oidcService.GetSessionCookieName(),
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL / time.Second),
	}
	http.SetCookie(w, cookie)

	redirectURL := c.determinePostLoginRedirect(r)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// HandleLogout handles user logout.
// GET /coordinator/oidc/logout
func (c *OIDCController) HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(c.oidcService.GetSessionCookieName())
	if err == nil && cookie.Value != "" {
		c.oidcService.DeleteSession(cookie.Value)
	}

	expiredCookie := &http.Cookie{
		Name:     c.oidcService.GetSessionCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   c.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
	http.SetCookie(w, expiredCookie)

	http.Redirect(w, r, defaultPostLoginRedirect, http.StatusFound)
}

func (c *OIDCController) determinePostLoginRedirect(r *http.Request) string {
	redirectTo := r.URL.Query().Get("redirect_to")
	if !isSafeRedirectPath(redirectTo) {
		return defaultPostLoginRedirect
	}
	return redirectTo
}
