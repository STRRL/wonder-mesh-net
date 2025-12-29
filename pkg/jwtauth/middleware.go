package jwtauth

import (
	"context"
	"net/http"
	"strings"
)

// contextKey is a type for context keys.
type contextKey string

// Context keys for JWT claims.
const (
	ContextKeyClaims contextKey = "jwt_claims"
)

// ClaimsFromContext retrieves JWT claims from the request context.
func ClaimsFromContext(ctx context.Context) *Claims {
	if claims, ok := ctx.Value(ContextKeyClaims).(*Claims); ok {
		return claims
	}
	return nil
}

// Middleware creates an HTTP middleware that validates JWT tokens.
// If the token is valid, the claims are added to the request context.
// If the token is invalid or missing, the request is rejected with 401.
func Middleware(validator *Validator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				http.Error(w, "authorization required", http.StatusUnauthorized)
				return
			}

			claims, err := validator.Validate(token)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ContextKeyClaims, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalMiddleware creates an HTTP middleware that validates JWT tokens
// but allows requests without tokens to proceed.
// If a token is present and valid, claims are added to the context.
// If a token is present but invalid, the request is rejected.
// If no token is present, the request proceeds without claims.
func OptionalMiddleware(validator *Validator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			claims, err := validator.Validate(token)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ContextKeyClaims, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return parts[1]
}
