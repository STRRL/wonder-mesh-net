package service

import "errors"

// OIDC service errors.
var (
	ErrProviderNotFound   = errors.New("provider not found")
	ErrInvalidRedirectURI = errors.New("invalid redirect URI: must be same origin")
	ErrInvalidState       = errors.New("invalid or expired state")
	ErrStateExpired       = errors.New("state expired")
	ErrTokenExchange      = errors.New("token exchange failed")
	ErrInvalidIDToken     = errors.New("invalid ID token")
	ErrMissingCode        = errors.New("missing authorization code")
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionExpired     = errors.New("session expired")
)

// Worker service errors.
var (
	ErrInvalidToken = errors.New("invalid or expired token")
)
