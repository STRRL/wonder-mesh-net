package handlers

import "time"

// APIKeyResponse represents an API key in JSON responses.
type APIKeyResponse struct {
	ID         string     `json:"id"`
	Key        string     `json:"key,omitempty"`
	Name       string     `json:"name"`
	Scopes     string     `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// APIKeyListResponse represents the response for listing API keys.
type APIKeyListResponse struct {
	APIKeys []APIKeyResponse `json:"api_keys"`
}

// NodeResponse represents a node in JSON responses.
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
