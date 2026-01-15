package wondersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the Wonder Mesh Net SDK client for Workload Managers
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new SDK client
func NewClient(coordinatorURL, apiKey string) *Client {
	return &Client{
		baseURL: coordinatorURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Node represents a node in the mesh
type Node struct {
	ID        uint64   `json:"id"`
	Name      string   `json:"name"`
	Addresses []string `json:"ip_addresses"`
	Online    bool     `json:"online"`
	LastSeen  string   `json:"last_seen,omitempty"`
}

// ListNodes returns all nodes for a user session or API key.
// If token is provided, it is used as Bearer token; otherwise falls back to client's apiKey.
func (c *Client) ListNodes(ctx context.Context, token string) ([]Node, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/nodes", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	bearerToken := token
	if bearerToken == "" {
		bearerToken = c.apiKey
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Nodes []Node `json:"nodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Nodes, nil
}

// GetOnlineNodes returns only online nodes for a user session or API key.
func (c *Client) GetOnlineNodes(ctx context.Context, token string) ([]Node, error) {
	nodes, err := c.ListNodes(ctx, token)
	if err != nil {
		return nil, err
	}

	online := make([]Node, 0)
	for _, n := range nodes {
		if n.Online {
			online = append(online, n)
		}
	}
	return online, nil
}

// Health checks if the coordinator is healthy
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}

	return nil
}
