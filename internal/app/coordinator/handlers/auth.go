package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/store"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

// AuthHandler handles authentication-related requests.
type AuthHandler struct {
	publicURL    string
	oidcRegistry *oidc.Registry
	realmManager *headscale.RealmManager
	aclManager   *headscale.ACLManager
	sessionStore store.SessionStore
	userStore    store.UserStore
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(
	publicURL string,
	oidcRegistry *oidc.Registry,
	realmManager *headscale.RealmManager,
	aclManager *headscale.ACLManager,
	sessionStore store.SessionStore,
	userStore store.UserStore,
) *AuthHandler {
	return &AuthHandler{
		publicURL:    publicURL,
		oidcRegistry: oidcRegistry,
		realmManager: realmManager,
		aclManager:   aclManager,
		sessionStore: sessionStore,
		userStore:    userStore,
	}
}

// HandleProviders handles GET /auth/providers requests.
func (h *AuthHandler) HandleProviders(w http.ResponseWriter, r *http.Request) {
	providers := h.oidcRegistry.ListProviders()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"providers": providers,
	}); err != nil {
		slog.Error("failed to encode providers response", "error", err)
	}
}

// HandleLogin handles GET /auth/login requests.
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		http.Error(w, "provider parameter required", http.StatusBadRequest)
		return
	}

	provider, ok := h.oidcRegistry.GetProvider(providerName)
	if !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}

	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		redirectURI = h.publicURL + "/coordinator/auth/complete"
	} else if !isValidRedirectURI(h.publicURL, redirectURI) {
		http.Error(w, "invalid redirect_uri: must be same origin", http.StatusBadRequest)
		return
	}

	authState, err := h.oidcRegistry.CreateAuthState(ctx, redirectURI, providerName)
	if err != nil {
		http.Error(w, "failed to create auth state", http.StatusInternalServerError)
		return
	}

	authURL := provider.GetAuthURL(authState.State)

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback handles GET /auth/callback requests.
func (h *AuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	authState, ok := h.oidcRegistry.ValidateState(ctx, state)
	if !ok {
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	provider, ok := h.oidcRegistry.GetProvider(authState.ProviderName)
	if !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}

	userInfo, err := provider.ExchangeCode(ctx, code)
	if err != nil {
		slog.Error("failed to exchange code", "error", err)
		http.Error(w, "failed to exchange code", http.StatusInternalServerError)
		return
	}

	existingUser, err := h.userStore.GetByIssuerSubject(ctx, provider.Issuer(), userInfo.Subject)
	if err != nil {
		slog.Error("failed to check existing user", "error", err)
		http.Error(w, "failed to check user", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	var userID, realmName string

	if existingUser != nil {
		userID = existingUser.ID
		realmName = existingUser.HeadscaleUser

		existingUser.Email = userInfo.Email
		existingUser.Name = userInfo.Name
		existingUser.Picture = userInfo.Picture
		if err := h.userStore.Update(ctx, existingUser); err != nil {
			slog.Warn("failed to update user info", "error", err, "user_id", existingUser.ID)
		}
	} else {
		userID = headscale.GenerateRealmID()
		realmName = headscale.RealmName(userID)

		newUser := &store.User{
			ID:            userID,
			HeadscaleUser: realmName,
			Issuer:        provider.Issuer(),
			Subject:       userInfo.Subject,
			Email:         userInfo.Email,
			Name:          userInfo.Name,
			Picture:       userInfo.Picture,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := h.userStore.Create(ctx, newUser); err != nil {
			slog.Error("failed to create user", "error", err)
			http.Error(w, "failed to create user", http.StatusInternalServerError)
			return
		}
	}

	hsUser, err := h.realmManager.GetOrCreateRealm(ctx, realmName)
	if err != nil {
		slog.Error("failed to get/create realm", "error", err)
		http.Error(w, "failed to create realm", http.StatusInternalServerError)
		return
	}

	if err := h.aclManager.AddRealmToPolicy(ctx, hsUser.GetName()); err != nil {
		slog.Error("failed to update ACL policy", "error", err, "user", hsUser.GetName())
		http.Error(w, "failed to update ACL policy", http.StatusInternalServerError)
		return
	}

	sessionID, err := store.GenerateSessionID()
	if err != nil {
		slog.Error("failed to generate session ID", "error", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	sessionTTL := 7 * 24 * time.Hour
	expiresAt := now.Add(sessionTTL)
	session := &store.Session{
		ID:         sessionID,
		UserID:     userID,
		Issuer:     provider.Issuer(),
		Subject:    userInfo.Subject,
		CreatedAt:  now,
		ExpiresAt:  &expiresAt,
		LastUsedAt: now,
	}
	if err := h.sessionStore.Create(ctx, session); err != nil {
		slog.Error("failed to create session", "error", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	secure := strings.HasPrefix(h.publicURL, "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     "wonder_session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "wonder_user",
		Value:    realmName,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7,
	})

	http.Redirect(w, r, authState.RedirectURI, http.StatusFound)
}

// HandleComplete handles GET /auth/complete requests.
func (h *AuthHandler) HandleComplete(w http.ResponseWriter, r *http.Request) {
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
		slog.Error("failed to encode complete response", "error", err)
	}
}

// HandleCreateAuthKey handles POST /api/v1/authkey requests.
func (h *AuthHandler) HandleCreateAuthKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	sessionID := r.Header.Get("X-Session-Token")
	if sessionID == "" {
		http.Error(w, "session token required", http.StatusUnauthorized)
		return
	}

	session, err := h.sessionStore.Get(ctx, sessionID)
	if err != nil {
		slog.Error("failed to get session", "error", err)
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}
	if session == nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	if err := h.sessionStore.UpdateLastUsed(ctx, sessionID); err != nil {
		slog.Warn("failed to update session last used", "error", err, "session_id", sessionID)
	}

	user, err := h.userStore.Get(ctx, session.UserID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusUnauthorized)
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

	key, err := h.realmManager.CreateAuthKeyByName(ctx, user.HeadscaleUser, ttl, req.Reusable)
	if err != nil {
		slog.Error("failed to create auth key", "error", err)
		http.Error(w, "failed to create auth key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"key":        key.GetKey(),
		"expiration": key.GetExpiration().AsTime(),
		"reusable":   key.GetReusable(),
	}); err != nil {
		slog.Error("failed to encode authkey response", "error", err)
	}
}

// isValidRedirectURI validates that the redirect URI is same-origin as publicURL.
// This prevents open redirect attacks by comparing scheme and host explicitly.
func isValidRedirectURI(publicURL, redirectURI string) bool {
	parsedPublic, err := url.Parse(publicURL)
	if err != nil {
		return false
	}

	parsedRedirect, err := url.Parse(redirectURI)
	if err != nil {
		return false
	}

	return parsedRedirect.Scheme == parsedPublic.Scheme &&
		parsedRedirect.Host == parsedPublic.Host
}
