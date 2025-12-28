package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database/sqlc"
)

const keyLength = 32

// ErrAPIKeyNotFound is returned when an API key is not found.
var ErrAPIKeyNotFound = errors.New("api key not found")

// APIKey represents an API key for third-party integrations.
type APIKey struct {
	ID         string
	RealmID    string
	Name       string
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
}

// APIKeyWithSecret includes the plaintext key for display to user.
type APIKeyWithSecret struct {
	APIKey
	Key string
}

// APIKeyRepository is the interface for storing API keys.
type APIKeyRepository interface {
	Create(ctx context.Context, realmID, name string, expiresAt *time.Time) (*APIKeyWithSecret, error)
	Get(ctx context.Context, id string) (*APIKeyWithSecret, error)
	GetByKey(ctx context.Context, key string) (*APIKey, error)
	List(ctx context.Context, realmID string) ([]*APIKeyWithSecret, error)
	Delete(ctx context.Context, id, realmID string) error
	UpdateLastUsed(ctx context.Context, id string) error
}

// DBAPIKeyRepository is a database implementation of APIKeyRepository.
type DBAPIKeyRepository struct {
	queries *sqlc.Queries
}

// NewDBAPIKeyRepository creates a new database-backed API key repository.
func NewDBAPIKeyRepository(queries *sqlc.Queries) *DBAPIKeyRepository {
	return &DBAPIKeyRepository{queries: queries}
}

// Create creates a new API key.
func (s *DBAPIKeyRepository) Create(ctx context.Context, realmID, name string, expiresAt *time.Time) (*APIKeyWithSecret, error) {
	keyBytes := make([]byte, keyLength)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, err
	}
	key := hex.EncodeToString(keyBytes)

	id := uuid.New().String()
	now := time.Now()

	var expiresAtSQL sql.NullTime
	if expiresAt != nil {
		expiresAtSQL = sql.NullTime{Time: *expiresAt, Valid: true}
	}

	err := s.queries.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
		ID:        id,
		RealmID:   realmID,
		Name:      name,
		ApiKey:    key,
		ExpiresAt: expiresAtSQL,
	})
	if err != nil {
		return nil, err
	}

	return &APIKeyWithSecret{
		APIKey: APIKey{
			ID:        id,
			RealmID:   realmID,
			Name:      name,
			CreatedAt: now,
			ExpiresAt: expiresAt,
		},
		Key: key,
	}, nil
}

// GetByKey retrieves an API key by its key value.
func (s *DBAPIKeyRepository) GetByKey(ctx context.Context, key string) (*APIKey, error) {
	dbKey, err := s.queries.GetAPIKeyByKey(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	apiKey := dbAPIKeyToAPIKey(dbKey)

	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, nil
	}

	return apiKey, nil
}

// List lists all API keys for a realm.
func (s *DBAPIKeyRepository) List(ctx context.Context, realmID string) ([]*APIKeyWithSecret, error) {
	dbKeys, err := s.queries.ListAPIKeysByRealm(ctx, realmID)
	if err != nil {
		return nil, err
	}

	keys := make([]*APIKeyWithSecret, len(dbKeys))
	for i, dbKey := range dbKeys {
		keys[i] = dbAPIKeyToAPIKeyWithSecret(dbKey)
	}
	return keys, nil
}

// Delete deletes an API key.
func (s *DBAPIKeyRepository) Delete(ctx context.Context, id, realmID string) error {
	result, err := s.queries.DeleteAPIKeyByRealm(ctx, sqlc.DeleteAPIKeyByRealmParams{
		ID:      id,
		RealmID: realmID,
	})
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

// UpdateLastUsed updates the last used timestamp.
func (s *DBAPIKeyRepository) UpdateLastUsed(ctx context.Context, id string) error {
	return s.queries.UpdateAPIKeyLastUsed(ctx, id)
}

// Get retrieves an API key by ID.
func (s *DBAPIKeyRepository) Get(ctx context.Context, id string) (*APIKeyWithSecret, error) {
	dbKey, err := s.queries.GetAPIKey(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return dbAPIKeyToAPIKeyWithSecret(dbKey), nil
}

func dbAPIKeyToAPIKey(dbKey sqlc.ApiKey) *APIKey {
	apiKey := &APIKey{
		ID:        dbKey.ID,
		RealmID:   dbKey.RealmID,
		Name:      dbKey.Name,
		CreatedAt: dbKey.CreatedAt,
	}
	if dbKey.ExpiresAt.Valid {
		apiKey.ExpiresAt = &dbKey.ExpiresAt.Time
	}
	if dbKey.LastUsedAt.Valid {
		apiKey.LastUsedAt = &dbKey.LastUsedAt.Time
	}
	return apiKey
}

func dbAPIKeyToAPIKeyWithSecret(dbKey sqlc.ApiKey) *APIKeyWithSecret {
	return &APIKeyWithSecret{
		APIKey: *dbAPIKeyToAPIKey(dbKey),
		Key:    dbKey.ApiKey,
	}
}
