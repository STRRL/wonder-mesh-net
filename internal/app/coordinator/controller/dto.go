// Package controller provides HTTP handlers for the coordinator API.
package controller

import "time"

// APIKeyResponse represents an API key in JSON responses.
type APIKeyResponse struct {
	ID         string     `json:"id"`
	Key        string     `json:"key,omitempty"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// APIKeyListResponse represents the response for listing API keys.
type APIKeyListResponse struct {
	APIKeys []APIKeyResponse `json:"api_keys"`
}

// NodeResponse represents a mesh network node in JSON responses.
type NodeResponse struct {
	ID       uint64   `json:"id"`
	Name     string   `json:"name"`
	IPAddrs  []string `json:"ip_addresses"`
	Online   bool     `json:"online"`
	LastSeen string   `json:"last_seen,omitempty"`
}

// NodeListResponse represents the response for listing nodes.
type NodeListResponse struct {
	Nodes []NodeResponse `json:"nodes"`
	Count int            `json:"count"`
}

// JoinCredentialsResponse contains credentials for joining the mesh.
type JoinCredentialsResponse struct {
	AuthKey      string `json:"authkey"`
	HeadscaleURL string `json:"headscale_url"`
	User         string `json:"user"`
}

// DeviceCodeResponse represents the response from device code initiation.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// DeviceTokenResponse represents the response from device token polling.
type DeviceTokenResponse struct {
	Authkey      string `json:"authkey,omitempty"`
	HeadscaleURL string `json:"headscale_url,omitempty"`
	User         string `json:"user,omitempty"`
	Error        string `json:"error,omitempty"`
}
