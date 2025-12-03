package headscale

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a Headscale API client
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Headscale API client
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// User represents a Headscale user (tenant)
type User struct {
	ID            uint64    `json:"id,omitempty"`
	Name          string    `json:"name"`
	DisplayName   string    `json:"displayName,omitempty"`
	Email         string    `json:"email,omitempty"`
	ProviderID    string    `json:"providerId,omitempty"`
	Provider      string    `json:"provider,omitempty"`
	ProfilePicURL string    `json:"profilePicUrl,omitempty"`
	CreatedAt     time.Time `json:"createdAt,omitempty"`
}

// PreAuthKey represents a pre-authentication key
type PreAuthKey struct {
	ID         uint64    `json:"id,omitempty"`
	Key        string    `json:"key,omitempty"`
	User       *User     `json:"user,omitempty"`
	Reusable   bool      `json:"reusable"`
	Ephemeral  bool      `json:"ephemeral"`
	Used       bool      `json:"used"`
	ACLTags    []string  `json:"aclTags,omitempty"`
	Expiration time.Time `json:"expiration,omitempty"`
	CreatedAt  time.Time `json:"createdAt,omitempty"`
}

// Node represents a Headscale node
type Node struct {
	ID        uint64    `json:"id"`
	Name      string    `json:"name"`
	User      *User     `json:"user,omitempty"`
	IPAddress []string  `json:"ipAddresses,omitempty"`
	Online    bool      `json:"online"`
	LastSeen  time.Time `json:"lastSeen,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

// do performs an HTTP request with authentication
func (c *Client) do(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

// Health checks if Headscale is healthy
func (c *Client) Health(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodGet, "/api/v1/health", nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}
	return nil
}

// CreateUser creates a new user (tenant)
func (c *Client) CreateUser(ctx context.Context, name string) (*User, error) {
	reqBody := map[string]string{"name": name}

	resp, err := c.do(ctx, http.MethodPost, "/api/v1/user", reqBody)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create user: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		User *User `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.User, nil
}

// GetUser gets a user by name
func (c *Client) GetUser(ctx context.Context, name string) (*User, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/v1/user?name="+name, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get user: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Users []*User `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	for _, u := range result.Users {
		if u.Name == name {
			return u, nil
		}
	}
	return nil, nil
}

// ListUsers lists all users
func (c *Client) ListUsers(ctx context.Context) ([]*User, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/v1/user", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list users: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Users []*User `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Users, nil
}

// DeleteUser deletes a user by ID
func (c *Client) DeleteUser(ctx context.Context, id uint64) error {
	resp, err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/user/%d", id), nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete user: status %d, body: %s", resp.StatusCode, string(body))
	}
	return nil
}

// CreatePreAuthKeyRequest is the request for creating a pre-auth key
type CreatePreAuthKeyRequest struct {
	UserID     uint64    `json:"user"`
	Reusable   bool      `json:"reusable"`
	Ephemeral  bool      `json:"ephemeral"`
	Expiration time.Time `json:"expiration,omitempty"`
	ACLTags    []string  `json:"aclTags,omitempty"`
}

// CreatePreAuthKey creates a pre-authentication key for a user
func (c *Client) CreatePreAuthKey(ctx context.Context, req *CreatePreAuthKeyRequest) (*PreAuthKey, error) {
	resp, err := c.do(ctx, http.MethodPost, "/api/v1/preauthkey", req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create pre-auth key: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		PreAuthKey *PreAuthKey `json:"preAuthKey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.PreAuthKey, nil
}

// ListPreAuthKeys lists pre-auth keys for a user
func (c *Client) ListPreAuthKeys(ctx context.Context, userID uint64) ([]*PreAuthKey, error) {
	resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/preauthkey?user=%d", userID), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list pre-auth keys: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		PreAuthKeys []*PreAuthKey `json:"preAuthKeys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.PreAuthKeys, nil
}

// ListNodes lists all nodes (optionally filtered by user)
func (c *Client) ListNodes(ctx context.Context, userID *uint64) ([]*Node, error) {
	path := "/api/v1/node"
	if userID != nil {
		path = fmt.Sprintf("/api/v1/node?user=%d", *userID)
	}

	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list nodes: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Nodes []*Node `json:"nodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Nodes, nil
}

// GetPolicy gets the current ACL policy
func (c *Client) GetPolicy(ctx context.Context) (string, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/v1/policy", nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get policy: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Policy string `json:"policy"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Policy, nil
}

// SetPolicy sets the ACL policy
func (c *Client) SetPolicy(ctx context.Context, policy string) error {
	reqBody := map[string]string{"policy": policy}

	resp, err := c.do(ctx, http.MethodPut, "/api/v1/policy", reqBody)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to set policy: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}
