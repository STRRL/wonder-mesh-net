package keycloak

import (
	"context"
	"errors"
	"fmt"

	"github.com/Nerzal/gocloak/v13"
)

var (
	ErrUserNotFound           = errors.New("user not found")
	ErrServiceAccountNotFound = errors.New("service account not found")
	ErrClientNotFound         = errors.New("client not found")
)

// AdminClientConfig holds configuration for the Keycloak admin client.
type AdminClientConfig struct {
	URL          string
	Realm        string
	ClientID     string
	ClientSecret string
}

// AdminClient provides Keycloak Admin API operations.
type AdminClient struct {
	client      *gocloak.GoCloak
	config      AdminClientConfig
	accessToken string
}

// NewAdminClient creates a new Keycloak admin client.
func NewAdminClient(config AdminClientConfig) *AdminClient {
	client := gocloak.NewClient(config.URL)
	return &AdminClient{
		client: client,
		config: config,
	}
}

// Authenticate authenticates the admin client using client credentials.
func (c *AdminClient) Authenticate(ctx context.Context) error {
	token, err := c.client.LoginClient(ctx, c.config.ClientID, c.config.ClientSecret, c.config.Realm)
	if err != nil {
		return fmt.Errorf("login client: %w", err)
	}
	c.accessToken = token.AccessToken
	return nil
}

// GetUserByKeycloakSub retrieves a user by their Keycloak subject (user ID).
func (c *AdminClient) GetUserByKeycloakSub(ctx context.Context, keycloakSub string) (*gocloak.User, error) {
	user, err := c.client.GetUserByID(ctx, c.accessToken, c.config.Realm, keycloakSub)
	if err != nil {
		return nil, fmt.Errorf("get user by ID: %w", err)
	}
	return user, nil
}

// GetUserByEmail retrieves a user by email address.
func (c *AdminClient) GetUserByEmail(ctx context.Context, email string) (*gocloak.User, error) {
	users, err := c.client.GetUsers(ctx, c.accessToken, c.config.Realm, gocloak.GetUsersParams{
		Email: gocloak.StringP(email),
		Exact: gocloak.BoolP(true),
	})
	if err != nil {
		return nil, fmt.Errorf("get users by email: %w", err)
	}
	if len(users) == 0 {
		return nil, ErrUserNotFound
	}
	return users[0], nil
}

// ServiceAccountInfo represents a Keycloak service account.
type ServiceAccountInfo struct {
	ClientID     string
	ClientSecret string
	UserID       string
	Description  string
}

// CreateServiceAccount creates a new service account (confidential client) in Keycloak.
func (c *AdminClient) CreateServiceAccount(ctx context.Context, clientID, description string) (*ServiceAccountInfo, error) {
	serviceAccountEnabled := true
	publicClient := false
	directAccessGrantsEnabled := false

	clientRepresentation := gocloak.Client{
		ClientID:                  gocloak.StringP(clientID),
		Name:                      gocloak.StringP(description),
		Description:               gocloak.StringP(description),
		ServiceAccountsEnabled:    &serviceAccountEnabled,
		PublicClient:              &publicClient,
		DirectAccessGrantsEnabled: &directAccessGrantsEnabled,
	}

	createdClientID, err := c.client.CreateClient(ctx, c.accessToken, c.config.Realm, clientRepresentation)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	createdClient, err := c.client.GetClient(ctx, c.accessToken, c.config.Realm, createdClientID)
	if err != nil {
		return nil, fmt.Errorf("get created client: %w", err)
	}

	serviceAccountUser, err := c.client.GetClientServiceAccount(ctx, c.accessToken, c.config.Realm, createdClientID)
	if err != nil {
		return nil, fmt.Errorf("get service account user: %w", err)
	}

	return &ServiceAccountInfo{
		ClientID:     *createdClient.ClientID,
		ClientSecret: *createdClient.Secret,
		UserID:       *serviceAccountUser.ID,
		Description:  description,
	}, nil
}

// DeleteServiceAccount deletes a service account by its client ID.
func (c *AdminClient) DeleteServiceAccount(ctx context.Context, clientID string) error {
	clients, err := c.client.GetClients(ctx, c.accessToken, c.config.Realm, gocloak.GetClientsParams{
		ClientID: gocloak.StringP(clientID),
	})
	if err != nil {
		return fmt.Errorf("get clients: %w", err)
	}
	if len(clients) == 0 {
		return ErrClientNotFound
	}

	if err := c.client.DeleteClient(ctx, c.accessToken, c.config.Realm, *clients[0].ID); err != nil {
		return fmt.Errorf("delete client: %w", err)
	}

	return nil
}

// ListServiceAccounts lists all service accounts (clients with service accounts enabled).
func (c *AdminClient) ListServiceAccounts(ctx context.Context) ([]*ServiceAccountInfo, error) {
	clients, err := c.client.GetClients(ctx, c.accessToken, c.config.Realm, gocloak.GetClientsParams{})
	if err != nil {
		return nil, fmt.Errorf("get clients: %w", err)
	}

	var result []*ServiceAccountInfo
	for _, client := range clients {
		if client.ServiceAccountsEnabled != nil && *client.ServiceAccountsEnabled {
			serviceAccountUser, err := c.client.GetClientServiceAccount(ctx, c.accessToken, c.config.Realm, *client.ID)
			if err != nil {
				continue
			}

			info := &ServiceAccountInfo{
				ClientID: *client.ClientID,
				UserID:   *serviceAccountUser.ID,
			}
			if client.Description != nil {
				info.Description = *client.Description
			}
			result = append(result, info)
		}
	}

	return result, nil
}

// GetServiceAccountByClientID retrieves a service account by its client ID.
func (c *AdminClient) GetServiceAccountByClientID(ctx context.Context, clientID string) (*ServiceAccountInfo, error) {
	clients, err := c.client.GetClients(ctx, c.accessToken, c.config.Realm, gocloak.GetClientsParams{
		ClientID: gocloak.StringP(clientID),
	})
	if err != nil {
		return nil, fmt.Errorf("get clients: %w", err)
	}
	if len(clients) == 0 {
		return nil, ErrClientNotFound
	}

	client := clients[0]
	if client.ServiceAccountsEnabled == nil || !*client.ServiceAccountsEnabled {
		return nil, ErrServiceAccountNotFound
	}

	serviceAccountUser, err := c.client.GetClientServiceAccount(ctx, c.accessToken, c.config.Realm, *client.ID)
	if err != nil {
		return nil, fmt.Errorf("get service account user: %w", err)
	}

	info := &ServiceAccountInfo{
		ClientID: *client.ClientID,
		UserID:   *serviceAccountUser.ID,
	}
	if client.Description != nil {
		info.Description = *client.Description
	}

	return info, nil
}

// SetUserAttribute sets a custom attribute on a user.
func (c *AdminClient) SetUserAttribute(ctx context.Context, userID, key, value string) error {
	user, err := c.client.GetUserByID(ctx, c.accessToken, c.config.Realm, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if user.Attributes == nil {
		user.Attributes = &map[string][]string{}
	}
	(*user.Attributes)[key] = []string{value}

	if err := c.client.UpdateUser(ctx, c.accessToken, c.config.Realm, *user); err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	return nil
}

// GetUserAttribute gets a custom attribute from a user.
func (c *AdminClient) GetUserAttribute(ctx context.Context, userID, key string) (string, error) {
	user, err := c.client.GetUserByID(ctx, c.accessToken, c.config.Realm, userID)
	if err != nil {
		return "", fmt.Errorf("get user: %w", err)
	}

	if user.Attributes == nil {
		return "", nil
	}

	values, ok := (*user.Attributes)[key]
	if !ok || len(values) == 0 {
		return "", nil
	}

	return values[0], nil
}
