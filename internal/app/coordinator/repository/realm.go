package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database/sqlc"
)

// Realm represents a realm (project/namespace) in the coordinator.
type Realm struct {
	ID            string
	OwnerID       string
	HeadscaleUser string
	DisplayName   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// RealmRepository provides realm storage operations.
type RealmRepository struct {
	queries *sqlc.Queries
}

// NewRealmRepository creates a new RealmRepository.
func NewRealmRepository(queries *sqlc.Queries) *RealmRepository {
	return &RealmRepository{queries: queries}
}

// Create creates a new realm.
func (s *RealmRepository) Create(ctx context.Context, realm *Realm) error {
	return s.queries.CreateRealm(ctx, sqlc.CreateRealmParams{
		ID:            realm.ID,
		OwnerID:       realm.OwnerID,
		HeadscaleUser: realm.HeadscaleUser,
		DisplayName:   realm.DisplayName,
	})
}

// Get retrieves a realm by ID.
func (s *RealmRepository) Get(ctx context.Context, id string) (*Realm, error) {
	row, err := s.queries.GetRealm(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return dbRealmToRealm(row), nil
}

// GetByHeadscaleUser retrieves a realm by Headscale user.
func (s *RealmRepository) GetByHeadscaleUser(ctx context.Context, headscaleUser string) (*Realm, error) {
	row, err := s.queries.GetRealmByHeadscaleUser(ctx, headscaleUser)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return dbRealmToRealm(row), nil
}

// ListByOwner lists all realms owned by a user.
func (s *RealmRepository) ListByOwner(ctx context.Context, ownerID string) ([]*Realm, error) {
	rows, err := s.queries.ListRealmsByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	realms := make([]*Realm, len(rows))
	for i, row := range rows {
		realms[i] = dbRealmToRealm(row)
	}
	return realms, nil
}

// Update updates a realm.
func (s *RealmRepository) Update(ctx context.Context, realm *Realm) error {
	return s.queries.UpdateRealm(ctx, sqlc.UpdateRealmParams{
		DisplayName: realm.DisplayName,
		ID:          realm.ID,
	})
}

// Delete deletes a realm.
func (s *RealmRepository) Delete(ctx context.Context, id string) error {
	return s.queries.DeleteRealm(ctx, id)
}

// List lists all realms.
func (s *RealmRepository) List(ctx context.Context) ([]*Realm, error) {
	rows, err := s.queries.ListRealms(ctx)
	if err != nil {
		return nil, err
	}
	realms := make([]*Realm, len(rows))
	for i, row := range rows {
		realms[i] = dbRealmToRealm(row)
	}
	return realms, nil
}

func dbRealmToRealm(row sqlc.Realm) *Realm {
	return &Realm{
		ID:            row.ID,
		OwnerID:       row.OwnerID,
		HeadscaleUser: row.HeadscaleUser,
		DisplayName:   row.DisplayName,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
