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

// UserRepository defines the interface for user storage operations.
type UserRepository interface {
	Create(ctx context.Context, displayName string) (*User, error)
	Get(ctx context.Context, id string) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*User, error)
}

// DBUserRepository implements UserRepository using the database.
type DBUserRepository struct {
	queries *sqlc.Queries
}

// NewDBUserRepository creates a new DBUserRepository.
func NewDBUserRepository(queries *sqlc.Queries) *DBUserRepository {
	return &DBUserRepository{queries: queries}
}

// Create creates a new user.
func (s *DBUserRepository) Create(ctx context.Context, displayName string) (*User, error) {
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
func (s *DBUserRepository) Get(ctx context.Context, id string) (*User, error) {
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
func (s *DBUserRepository) Update(ctx context.Context, user *User) error {
	return s.queries.UpdateUser(ctx, sqlc.UpdateUserParams{
		DisplayName: user.DisplayName,
		ID:          user.ID,
	})
}

// Delete deletes a user.
func (s *DBUserRepository) Delete(ctx context.Context, id string) error {
	return s.queries.DeleteUser(ctx, id)
}

// List lists all users.
func (s *DBUserRepository) List(ctx context.Context) ([]*User, error) {
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
