package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database/sqlc"
)

// User represents a system user.
type User struct {
	ID          string
	KeycloakSub string
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// UserRepository provides user storage operations.
type UserRepository struct {
	queries *sqlc.Queries
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(queries *sqlc.Queries) *UserRepository {
	return &UserRepository{queries: queries}
}

// Create creates a new user.
func (r *UserRepository) Create(ctx context.Context, keycloakSub, displayName string) (*User, error) {
	id := uuid.New().String()

	err := r.queries.CreateUser(ctx, sqlc.CreateUserParams{
		ID:          id,
		KeycloakSub: keycloakSub,
		DisplayName: displayName,
	})
	if err != nil {
		return nil, err
	}

	return r.Get(ctx, id)
}

// Get retrieves a user by ID.
func (r *UserRepository) Get(ctx context.Context, id string) (*User, error) {
	row, err := r.queries.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &User{
		ID:          row.ID,
		KeycloakSub: row.KeycloakSub,
		DisplayName: row.DisplayName,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}, nil
}

// GetByKeycloakSub retrieves a user by Keycloak subject claim.
func (r *UserRepository) GetByKeycloakSub(ctx context.Context, keycloakSub string) (*User, error) {
	row, err := r.queries.GetUserByKeycloakSub(ctx, keycloakSub)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &User{
		ID:          row.ID,
		KeycloakSub: row.KeycloakSub,
		DisplayName: row.DisplayName,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}, nil
}

// Update updates a user.
func (r *UserRepository) Update(ctx context.Context, user *User) error {
	return r.queries.UpdateUser(ctx, sqlc.UpdateUserParams{
		DisplayName: user.DisplayName,
		ID:          user.ID,
	})
}

// Delete deletes a user.
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	return r.queries.DeleteUser(ctx, id)
}

// List lists all users.
func (r *UserRepository) List(ctx context.Context) ([]*User, error) {
	rows, err := r.queries.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]*User, len(rows))
	for i, row := range rows {
		users[i] = &User{
			ID:          row.ID,
			KeycloakSub: row.KeycloakSub,
			DisplayName: row.DisplayName,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		}
	}
	return users, nil
}
