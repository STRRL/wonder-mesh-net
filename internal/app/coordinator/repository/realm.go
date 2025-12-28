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

// RealmRepository is the interface for storing realms.
type RealmRepository interface {
	Create(ctx context.Context, realm *Realm) error
	Get(ctx context.Context, id string) (*Realm, error)
	GetByHeadscaleUser(ctx context.Context, headscaleUser string) (*Realm, error)
	ListByOwner(ctx context.Context, ownerID string) ([]*Realm, error)
	Update(ctx context.Context, realm *Realm) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Realm, error)
}

// DBRealmRepository is a database implementation of RealmRepository.
type DBRealmRepository struct {
	queries *sqlc.Queries
}

// NewDBRealmRepository creates a new database-backed realm repository.
func NewDBRealmRepository(queries *sqlc.Queries) *DBRealmRepository {
	return &DBRealmRepository{queries: queries}
}

// Create creates a new realm.
func (s *DBRealmRepository) Create(ctx context.Context, realm *Realm) error {
	return s.queries.CreateRealm(ctx, sqlc.CreateRealmParams{
		ID:            realm.ID,
		OwnerID:       realm.OwnerID,
		HeadscaleUser: realm.HeadscaleUser,
		DisplayName:   realm.DisplayName,
	})
}

// Get retrieves a realm by ID.
func (s *DBRealmRepository) Get(ctx context.Context, id string) (*Realm, error) {
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
func (s *DBRealmRepository) GetByHeadscaleUser(ctx context.Context, headscaleUser string) (*Realm, error) {
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
func (s *DBRealmRepository) ListByOwner(ctx context.Context, ownerID string) ([]*Realm, error) {
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
func (s *DBRealmRepository) Update(ctx context.Context, realm *Realm) error {
	return s.queries.UpdateRealm(ctx, sqlc.UpdateRealmParams{
		DisplayName: realm.DisplayName,
		ID:          realm.ID,
	})
}

// Delete deletes a realm.
func (s *DBRealmRepository) Delete(ctx context.Context, id string) error {
	return s.queries.DeleteRealm(ctx, id)
}

// List lists all realms.
func (s *DBRealmRepository) List(ctx context.Context) ([]*Realm, error) {
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
