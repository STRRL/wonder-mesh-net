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

// OIDCIdentityRepository provides OIDC identity storage operations.
type OIDCIdentityRepository struct {
	queries *sqlc.Queries
}

// NewOIDCIdentityRepository creates a new database-backed OIDC identity repository.
func NewOIDCIdentityRepository(queries *sqlc.Queries) *OIDCIdentityRepository {
	return &OIDCIdentityRepository{queries: queries}
}

// Create creates a new OIDC identity.
func (s *OIDCIdentityRepository) Create(ctx context.Context, identity *OIDCIdentity) error {
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
func (s *OIDCIdentityRepository) Get(ctx context.Context, id string) (*OIDCIdentity, error) {
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
func (s *OIDCIdentityRepository) GetByIssuerSubject(ctx context.Context, issuer, subject string) (*OIDCIdentity, error) {
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
func (s *OIDCIdentityRepository) ListByUser(ctx context.Context, userID string) ([]*OIDCIdentity, error) {
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
func (s *OIDCIdentityRepository) Update(ctx context.Context, identity *OIDCIdentity) error {
	return s.queries.UpdateOIDCIdentity(ctx, sqlc.UpdateOIDCIdentityParams{
		Email:   identity.Email,
		Name:    identity.Name,
		Picture: identity.Picture,
		ID:      identity.ID,
	})
}

// Delete deletes an OIDC identity.
func (s *OIDCIdentityRepository) Delete(ctx context.Context, id string) error {
	return s.queries.DeleteOIDCIdentity(ctx, id)
}

// DeleteByUser deletes all OIDC identities for a user.
func (s *OIDCIdentityRepository) DeleteByUser(ctx context.Context, userID string) error {
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
