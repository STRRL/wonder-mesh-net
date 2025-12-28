package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database/sqlc"
)

// User represents a system user.
type User struct {
	ID          string
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
func (s *UserRepository) Create(ctx context.Context, displayName string) (*User, error) {
	id := uuid.New().String()

	err := s.queries.CreateUser(ctx, sqlc.CreateUserParams{
		ID:          id,
		DisplayName: displayName,
	})
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Get retrieves a user by ID.
func (s *UserRepository) Get(ctx context.Context, id string) (*User, error) {
	row, err := s.queries.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}

	return &User{
		ID:          row.ID,
		DisplayName: row.DisplayName,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}, nil
}

// Update updates a user.
func (s *UserRepository) Update(ctx context.Context, user *User) error {
	return s.queries.UpdateUser(ctx, sqlc.UpdateUserParams{
		DisplayName: user.DisplayName,
		ID:          user.ID,
	})
}

// Delete deletes a user.
func (s *UserRepository) Delete(ctx context.Context, id string) error {
	return s.queries.DeleteUser(ctx, id)
}

// List lists all users.
func (s *UserRepository) List(ctx context.Context) ([]*User, error) {
	rows, err := s.queries.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]*User, len(rows))
	for i, row := range rows {
		users[i] = &User{
			ID:          row.ID,
			DisplayName: row.DisplayName,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		}
	}
	return users, nil
}
