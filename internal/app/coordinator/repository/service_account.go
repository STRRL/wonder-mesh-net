package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database/sqlc"
)

// ServiceAccount represents a service account in the coordinator.
type ServiceAccount struct {
	ID               string
	WonderNetID      string
	KeycloakClientID string
	Name             string
	CreatedAt        time.Time
}

// ServiceAccountRepository provides service account storage operations.
type ServiceAccountRepository struct {
	queries *sqlc.Queries
}

// NewServiceAccountRepository creates a new ServiceAccountRepository.
func NewServiceAccountRepository(queries *sqlc.Queries) *ServiceAccountRepository {
	return &ServiceAccountRepository{queries: queries}
}

// Create creates a new service account record.
func (r *ServiceAccountRepository) Create(ctx context.Context, wonderNetID, keycloakClientID, name string) (*ServiceAccount, error) {
	id := uuid.New().String()
	err := r.queries.CreateServiceAccount(ctx, sqlc.CreateServiceAccountParams{
		ID:               id,
		WonderNetID:      wonderNetID,
		KeycloakClientID: keycloakClientID,
		Name:             name,
	})
	if err != nil {
		return nil, err
	}
	return &ServiceAccount{
		ID:               id,
		WonderNetID:      wonderNetID,
		KeycloakClientID: keycloakClientID,
		Name:             name,
		CreatedAt:        time.Now(),
	}, nil
}

// GetByClientID retrieves a service account by its Keycloak client ID.
func (r *ServiceAccountRepository) GetByClientID(ctx context.Context, keycloakClientID string) (*ServiceAccount, error) {
	row, err := r.queries.GetServiceAccountByClientID(ctx, keycloakClientID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return dbServiceAccountToServiceAccount(row), nil
}

// ListByWonderNet lists all service accounts for a wonder net.
func (r *ServiceAccountRepository) ListByWonderNet(ctx context.Context, wonderNetID string) ([]*ServiceAccount, error) {
	rows, err := r.queries.ListServiceAccountsByWonderNet(ctx, wonderNetID)
	if err != nil {
		return nil, err
	}
	accounts := make([]*ServiceAccount, len(rows))
	for i, row := range rows {
		accounts[i] = dbServiceAccountToServiceAccount(row)
	}
	return accounts, nil
}

// Delete deletes a service account by its Keycloak client ID.
func (r *ServiceAccountRepository) Delete(ctx context.Context, keycloakClientID string) error {
	return r.queries.DeleteServiceAccount(ctx, keycloakClientID)
}

func dbServiceAccountToServiceAccount(row sqlc.ServiceAccount) *ServiceAccount {
	return &ServiceAccount{
		ID:               row.ID,
		WonderNetID:      row.WonderNetID,
		KeycloakClientID: row.KeycloakClientID,
		Name:             row.Name,
		CreatedAt:        row.CreatedAt,
	}
}
