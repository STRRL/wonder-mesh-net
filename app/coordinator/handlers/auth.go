package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/headscale"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

// AuthHandler handles authentication-related requests.
type AuthHandler struct {
	publicURL     string
	oidcRegistry  *oidc.Registry
	tenantManager *headscale.TenantManager
	aclManager    *headscale.ACLManager
	sessionStore  oidc.SessionStore
	userStore     oidc.UserStore
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(
	publicURL string,
	oidcRegistry *oidc.Registry,
	tenantManager *headscale.TenantManager,
	aclManager *headscale.ACLManager,
	sessionStore oidc.SessionStore,
	userStore oidc.UserStore,
) *AuthHandler {
	return &AuthHandler{
		publicURL:     publicURL,
		oidcRegistry:  oidcRegistry,
		tenantManager: tenantManager,
		aclManager:    aclManager,
		sessionStore:  sessionStore,
		userStore:     userStore,
	}
}

// HandleProviders handles GET /auth/providers requests.
func (h *AuthHandler) HandleProviders(w http.ResponseWriter, r *http.Request) {
	providers := h.oidcRegistry.ListProviders()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"providers": providers,
	})
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
		redirectURI = h.publicURL + "/auth/complete"
	}

	authState, err := h.oidcRegistry.CreateAuthState(ctx, redirectURI, providerName)
	if err != nil {
		http.Error(w, "failed to create auth state", http.StatusInternalServerError)
		return
	}

	callbackURL := h.publicURL + "/auth/callback?provider=" + providerName
	authURL := provider.GetAuthURL(callbackURL, authState.State)

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

	callbackURL := h.publicURL + "/auth/callback?provider=" + authState.ProviderName
	userInfo, err := provider.ExchangeCode(ctx, code, callbackURL)
	if err != nil {
		log.Printf("Failed to exchange code: %v", err)
		http.Error(w, "failed to exchange code", http.StatusInternalServerError)
		return
	}

	hsUser, err := h.tenantManager.GetOrCreateTenant(ctx, provider.Issuer(), userInfo.Subject)
	if err != nil {
		log.Printf("Failed to get/create tenant: %v", err)
		http.Error(w, "failed to create tenant", http.StatusInternalServerError)
		return
	}

	if err := h.aclManager.AddTenantToPolicy(ctx, hsUser.GetName()); err != nil {
		log.Printf("Warning: failed to update ACL policy: %v", err)
	}

	userID := headscale.DeriveTenantID(provider.Issuer(), userInfo.Subject)
	existingUser, err := h.userStore.GetByIssuerSubject(ctx, provider.Issuer(), userInfo.Subject)
	if err != nil {
		log.Printf("Failed to check existing user: %v", err)
		http.Error(w, "failed to check user", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	if existingUser == nil {
		newUser := &oidc.User{
			ID:              userID,
			HeadscaleUser:   hsUser.GetName(),
			HeadscaleUserID: hsUser.GetId(),
			Issuer:          provider.Issuer(),
			Subject:         userInfo.Subject,
			Email:           userInfo.Email,
			Name:            userInfo.Name,
			Picture:         userInfo.Picture,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := h.userStore.Create(ctx, newUser); err != nil {
			log.Printf("Failed to create user: %v", err)
			http.Error(w, "failed to create user", http.StatusInternalServerError)
			return
		}
	} else {
		existingUser.Email = userInfo.Email
		existingUser.Name = userInfo.Name
		existingUser.Picture = userInfo.Picture
		if err := h.userStore.Update(ctx, existingUser); err != nil {
			log.Printf("Warning: failed to update user info: %v", err)
		}
	}

	sessionID, err := oidc.GenerateSessionID()
	if err != nil {
		log.Printf("Failed to generate session ID: %v", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	session := &oidc.Session{
		ID:         sessionID,
		UserID:     userID,
		Issuer:     provider.Issuer(),
		Subject:    userInfo.Subject,
		CreatedAt:  now,
		LastUsedAt: now,
	}
	if err := h.sessionStore.Create(ctx, session); err != nil {
		log.Printf("Failed to create session: %v", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	redirectURL := authState.RedirectURI + "?session=" + sessionID + "&user=" + hsUser.GetName()
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// HandleComplete handles GET /auth/complete requests.
func (h *AuthHandler) HandleComplete(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session")
	user := r.URL.Query().Get("user")

	if session == "" {
		http.Error(w, "missing session", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"session": session,
		"user":    user,
	})
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
		log.Printf("Failed to get session: %v", err)
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}
	if session == nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	_ = h.sessionStore.UpdateLastUsed(ctx, sessionID)

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

	key, err := h.tenantManager.CreateAuthKey(ctx, user.HeadscaleUserID, ttl, req.Reusable)
	if err != nil {
		log.Printf("Failed to create auth key: %v", err)
		http.Error(w, "failed to create auth key", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"key":        key.GetKey(),
		"expiration": key.GetExpiration().AsTime(),
		"reusable":   key.GetReusable(),
	})
}
