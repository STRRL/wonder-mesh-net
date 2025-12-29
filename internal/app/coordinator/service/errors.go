package service

import "errors"

// OIDC service errors.
var (
	ErrProviderNotFound   = errors.New("provider not found")
	ErrInvalidRedirectURI = errors.New("invalid redirect URI: must be same origin")
	ErrInvalidState       = errors.New("invalid or expired state")
)

// Worker service errors.
var (
	ErrInvalidToken = errors.New("invalid or expired token")
)
