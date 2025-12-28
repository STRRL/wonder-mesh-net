package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database/sqlc"
)

// OIDCIdentity represents an OIDC identity linked to a user.
type OIDCIdentity struct {
	ID        string
	UserID    string
	Issuer    string
	Subject   string
	Email     string
	Name      string
	Picture   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// OIDCIdentityRepository is the interface for storing OIDC identities.
type OIDCIdentityRepository interface {
	Create(ctx context.Context, identity *OIDCIdentity) error
	Get(ctx context.Context, id string) (*OIDCIdentity, error)
	GetByIssuerSubject(ctx context.Context, issuer, subject string) (*OIDCIdentity, error)
	ListByUser(ctx context.Context, userID string) ([]*OIDCIdentity, error)
	Update(ctx context.Context, identity *OIDCIdentity) error
	Delete(ctx context.Context, id string) error
	DeleteByUser(ctx context.Context, userID string) error
}

// DBOIDCIdentityRepository is a database implementation of OIDCIdentityRepository.
type DBOIDCIdentityRepository struct {
	queries *sqlc.Queries
}

// NewDBOIDCIdentityRepository creates a new database-backed OIDC identity repository.
func NewDBOIDCIdentityRepository(queries *sqlc.Queries) *DBOIDCIdentityRepository {
	return &DBOIDCIdentityRepository{queries: queries}
}

// Create creates a new OIDC identity.
func (s *DBOIDCIdentityRepository) Create(ctx context.Context, identity *OIDCIdentity) error {
	if identity.ID == "" {
		identity.ID = uuid.New().String()
	}
	return s.queries.CreateOIDCIdentity(ctx, sqlc.CreateOIDCIdentityParams{
		ID:      identity.ID,
		UserID:  identity.UserID,
		Issuer:  identity.Issuer,
		Subject: identity.Subject,
		Email:   identity.Email,
		Name:    identity.Name,
		Picture: identity.Picture,
	})
}

// Get retrieves an OIDC identity by ID.
func (s *DBOIDCIdentityRepository) Get(ctx context.Context, id string) (*OIDCIdentity, error) {
	row, err := s.queries.GetOIDCIdentity(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return dbOIDCIdentityToOIDCIdentity(row), nil
}

// GetByIssuerSubject retrieves an OIDC identity by issuer and subject.
func (s *DBOIDCIdentityRepository) GetByIssuerSubject(ctx context.Context, issuer, subject string) (*OIDCIdentity, error) {
	row, err := s.queries.GetOIDCIdentityByIssuerSubject(ctx, sqlc.GetOIDCIdentityByIssuerSubjectParams{
		Issuer:  issuer,
		Subject: subject,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return dbOIDCIdentityToOIDCIdentity(row), nil
}

// ListByUser lists all OIDC identities for a user.
func (s *DBOIDCIdentityRepository) ListByUser(ctx context.Context, userID string) ([]*OIDCIdentity, error) {
	rows, err := s.queries.ListOIDCIdentitiesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	identities := make([]*OIDCIdentity, len(rows))
	for i, row := range rows {
		identities[i] = dbOIDCIdentityToOIDCIdentity(row)
	}
	return identities, nil
}

// Update updates an OIDC identity.
func (s *DBOIDCIdentityRepository) Update(ctx context.Context, identity *OIDCIdentity) error {
	return s.queries.UpdateOIDCIdentity(ctx, sqlc.UpdateOIDCIdentityParams{
		Email:   identity.Email,
		Name:    identity.Name,
		Picture: identity.Picture,
		ID:      identity.ID,
	})
}

// Delete deletes an OIDC identity.
func (s *DBOIDCIdentityRepository) Delete(ctx context.Context, id string) error {
	return s.queries.DeleteOIDCIdentity(ctx, id)
}

// DeleteByUser deletes all OIDC identities for a user.
func (s *DBOIDCIdentityRepository) DeleteByUser(ctx context.Context, userID string) error {
	return s.queries.DeleteOIDCIdentitiesByUser(ctx, userID)
}

func dbOIDCIdentityToOIDCIdentity(row sqlc.OidcIdentity) *OIDCIdentity {
	return &OIDCIdentity{
		ID:        row.ID,
		UserID:    row.UserID,
		Issuer:    row.Issuer,
		Subject:   row.Subject,
		Email:     row.Email,
		Name:      row.Name,
		Picture:   row.Picture,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
