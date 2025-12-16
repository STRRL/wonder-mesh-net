package oidc

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/database"
)

// User represents a local user record
type User struct {
	ID            string
	HeadscaleUser string
	Issuer        string
	Subject       string
	Email         string
	Name          string
	Picture       string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// UserStore is the interface for storing users
type UserStore interface {
	Create(ctx context.Context, user *User) error
	Get(ctx context.Context, id string) (*User, error)
	GetByHeadscaleUser(ctx context.Context, headscaleUser string) (*User, error)
	GetByIssuerSubject(ctx context.Context, issuer, subject string) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id string) error
}

// DBUserStore is a database implementation of UserStore
type DBUserStore struct {
	queries *database.Queries
}

// NewDBUserStore creates a new database-backed user store
func NewDBUserStore(queries *database.Queries) *DBUserStore {
	return &DBUserStore{queries: queries}
}

func (s *DBUserStore) Create(ctx context.Context, user *User) error {
	return s.queries.CreateUser(ctx, database.CreateUserParams{
		ID:            user.ID,
		HeadscaleUser: user.HeadscaleUser,
		Issuer:        user.Issuer,
		Subject:       user.Subject,
		Email:         sql.NullString{String: user.Email, Valid: user.Email != ""},
		Name:          sql.NullString{String: user.Name, Valid: user.Name != ""},
		Picture:       sql.NullString{String: user.Picture, Valid: user.Picture != ""},
		CreatedAt:     user.CreatedAt,
		UpdatedAt:     user.UpdatedAt,
	})
}

func (s *DBUserStore) Get(ctx context.Context, id string) (*User, error) {
	dbUser, err := s.queries.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return dbUserToUser(dbUser), nil
}

func (s *DBUserStore) GetByHeadscaleUser(ctx context.Context, headscaleUser string) (*User, error) {
	dbUser, err := s.queries.GetUserByHeadscaleUser(ctx, headscaleUser)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return dbUserToUser(dbUser), nil
}

func (s *DBUserStore) GetByIssuerSubject(ctx context.Context, issuer, subject string) (*User, error) {
	dbUser, err := s.queries.GetUserByIssuerSubject(ctx, database.GetUserByIssuerSubjectParams{
		Issuer:  issuer,
		Subject: subject,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return dbUserToUser(dbUser), nil
}

func (s *DBUserStore) Update(ctx context.Context, user *User) error {
	return s.queries.UpdateUser(ctx, database.UpdateUserParams{
		Email:     sql.NullString{String: user.Email, Valid: user.Email != ""},
		Name:      sql.NullString{String: user.Name, Valid: user.Name != ""},
		Picture:   sql.NullString{String: user.Picture, Valid: user.Picture != ""},
		UpdatedAt: time.Now(),
		ID:        user.ID,
	})
}

func (s *DBUserStore) Delete(ctx context.Context, id string) error {
	return s.queries.DeleteUser(ctx, id)
}

func dbUserToUser(dbUser database.User) *User {
	return &User{
		ID:            dbUser.ID,
		HeadscaleUser: dbUser.HeadscaleUser,
		Issuer:        dbUser.Issuer,
		Subject:       dbUser.Subject,
		Email:         dbUser.Email.String,
		Name:          dbUser.Name.String,
		Picture:       dbUser.Picture.String,
		CreatedAt:     dbUser.CreatedAt,
		UpdatedAt:     dbUser.UpdatedAt,
	}
}
