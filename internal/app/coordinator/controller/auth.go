package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// AuthController handles OIDC authentication flows.
type AuthController struct {
	oidcService  *service.OIDCService
	authService  *service.AuthService
	realmService *service.RealmService
	publicURL    string
}

// NewAuthController creates a new AuthController.
func NewAuthController(
	oidcService *service.OIDCService,
	authService *service.AuthService,
	realmService *service.RealmService,
	publicURL string,
) *AuthController {
	return &AuthController{
		oidcService:  oidcService,
		authService:  authService,
		realmService: realmService,
		publicURL:    publicURL,
	}
}

// HandleProviders handles GET /auth/providers requests.
func (c *AuthController) HandleProviders(w http.ResponseWriter, r *http.Request) {
	providers := c.oidcService.ListProviders()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"providers": providers,
	}); err != nil {
		slog.Error("encode providers response", "error", err)
	}
}

// HandleLogin handles GET /auth/login requests.
func (c *AuthController) HandleLogin(w http.ResponseWriter, r *http.Request) {
	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		http.Error(w, "provider parameter required", http.StatusBadRequest)
		return
	}

	redirectURI := r.URL.Query().Get("redirect_uri")

	authURL, err := c.oidcService.InitiateLogin(r.Context(), providerName, redirectURI, c.publicURL)
	if err != nil {
		switch err {
		case service.ErrProviderNotFound:
			http.Error(w, "unknown provider", http.StatusBadRequest)
		case service.ErrInvalidRedirectURI:
			http.Error(w, "invalid redirect_uri: must be same origin", http.StatusBadRequest)
		default:
			http.Error(w, "create auth state", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback handles GET /auth/callback requests.
func (c *AuthController) HandleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	result, redirectURI, err := c.oidcService.CompleteCallback(ctx, code, state)
	if err != nil {
		slog.Error("complete callback", "error", err)
		switch err {
		case service.ErrInvalidState:
			http.Error(w, "invalid or expired state", http.StatusBadRequest)
		case service.ErrProviderNotFound:
			http.Error(w, "unknown provider", http.StatusBadRequest)
		default:
			http.Error(w, "authentication error", http.StatusInternalServerError)
		}
		return
	}

	session, err := c.authService.CreateSession(ctx, result.UserID, 7*24*time.Hour)
	if err != nil {
		slog.Error("create session", "error", err)
		http.Error(w, "create session", http.StatusInternalServerError)
		return
	}

	secure := strings.HasPrefix(c.publicURL, "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     "wonder_session",
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "wonder_user",
		Value:    result.HeadscaleUser,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7,
	})

	http.Redirect(w, r, redirectURI, http.StatusFound)
}

// HandleComplete handles GET /auth/complete requests.
func (c *AuthController) HandleComplete(w http.ResponseWriter, r *http.Request) {
	var session, user string

	if cookie, err := r.Cookie("wonder_session"); err == nil {
		session = cookie.Value
	}
	if cookie, err := r.Cookie("wonder_user"); err == nil {
		user = cookie.Value
	}

	if session == "" {
		http.Error(w, "missing session", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"session": session,
		"user":    user,
	}); err != nil {
		slog.Error("encode complete response", "error", err)
	}
}

// HandleCreateAuthKey handles POST /api/v1/authkey requests.
func (c *AuthController) HandleCreateAuthKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	realm, err := c.authService.GetRealmFromRequest(ctx, r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		TTL      string `json:"ttl"`
		Reusable bool   `json:"reusable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ttl := 24 * time.Hour
	if req.TTL != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			http.Error(w, "invalid TTL format", http.StatusBadRequest)
			return
		}
		ttl = parsed
	}

	key, err := c.realmService.CreateAuthKey(ctx, realm, ttl, req.Reusable)
	if err != nil {
		slog.Error("create auth key", "error", err)
		http.Error(w, "create auth key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"key":      key,
		"reusable": req.Reusable,
	}); err != nil {
		slog.Error("encode authkey response", "error", err)
	}
}
