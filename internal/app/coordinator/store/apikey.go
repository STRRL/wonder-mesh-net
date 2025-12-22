package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database"
)

const keyLength = 32

// ErrAPIKeyNotFound is returned when an API key is not found.
var ErrAPIKeyNotFound = errors.New("api key not found")

// HasScope checks if the given scope is present in a comma-separated scope string.
// Performs exact match, not substring match.
func HasScope(scopes, target string) bool {
	for _, s := range strings.Split(scopes, ",") {
		if strings.TrimSpace(s) == target {
			return true
		}
	}
	return false
}

// APIKey represents an API key for third-party integrations
type APIKey struct {
	// ID is the unique identifier (UUID)
	ID string
	// UserID is the owner's user ID
	UserID string
	// Name is a human-readable label for the key
	Name string
	// Scopes defines permissions (e.g., "nodes:read")
	Scopes string
	// CreatedAt is when the key was created
	CreatedAt time.Time
	// ExpiresAt is the optional expiration time (nil = never expires)
	ExpiresAt *time.Time
	// LastUsedAt is when the key was last used for authentication
	LastUsedAt *time.Time
}

// APIKeyWithSecret includes the plaintext key for display to user
type APIKeyWithSecret struct {
	APIKey
	// Key is the plaintext API key (64-char hex string, 32 bytes)
	Key string
}

// APIKeyStore is the interface for storing API keys
type APIKeyStore interface {
	Create(ctx context.Context, userID, name, scopes string, expiresAt *time.Time) (*APIKeyWithSecret, error)
	Get(ctx context.Context, id string) (*APIKeyWithSecret, error)
	GetByKey(ctx context.Context, key string) (*APIKey, error)
	List(ctx context.Context, userID string) ([]*APIKeyWithSecret, error)
	Delete(ctx context.Context, id, userID string) error
	UpdateLastUsed(ctx context.Context, id string) error
}

// DBAPIKeyStore is a database implementation of APIKeyStore
type DBAPIKeyStore struct {
	queries *database.Queries
}

// NewDBAPIKeyStore creates a new database-backed API key store
func NewDBAPIKeyStore(queries *database.Queries) *DBAPIKeyStore {
	return &DBAPIKeyStore{queries: queries}
}

func (s *DBAPIKeyStore) Create(ctx context.Context, userID, name, scopes string, expiresAt *time.Time) (*APIKeyWithSecret, error) {
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

	err := s.queries.CreateAPIKey(ctx, database.CreateAPIKeyParams{
		ID:        id,
		UserID:    userID,
		Name:      name,
		ApiKey:    key,
		Scopes:    scopes,
		CreatedAt: now,
		ExpiresAt: expiresAtSQL,
	})
	if err != nil {
		return nil, err
	}

	return &APIKeyWithSecret{
		APIKey: APIKey{
			ID:        id,
			UserID:    userID,
			Name:      name,
			Scopes:    scopes,
			CreatedAt: now,
			ExpiresAt: expiresAt,
		},
		Key: key,
	}, nil
}

func (s *DBAPIKeyStore) GetByKey(ctx context.Context, key string) (*APIKey, error) {
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

func (s *DBAPIKeyStore) List(ctx context.Context, userID string) ([]*APIKeyWithSecret, error) {
	dbKeys, err := s.queries.ListAPIKeysByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	keys := make([]*APIKeyWithSecret, len(dbKeys))
	for i, dbKey := range dbKeys {
		keys[i] = dbAPIKeyToAPIKeyWithSecret(dbKey)
	}
	return keys, nil
}

func (s *DBAPIKeyStore) Delete(ctx context.Context, id, userID string) error {
	result, err := s.queries.DeleteAPIKeyByUser(ctx, database.DeleteAPIKeyByUserParams{
		ID:     id,
		UserID: userID,
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

func (s *DBAPIKeyStore) UpdateLastUsed(ctx context.Context, id string) error {
	return s.queries.UpdateAPIKeyLastUsed(ctx, database.UpdateAPIKeyLastUsedParams{
		LastUsedAt: sql.NullTime{Time: time.Now(), Valid: true},
		ID:         id,
	})
}

func (s *DBAPIKeyStore) Get(ctx context.Context, id string) (*APIKeyWithSecret, error) {
	dbKey, err := s.queries.GetAPIKey(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return dbAPIKeyToAPIKeyWithSecret(dbKey), nil
}

func dbAPIKeyToAPIKey(dbKey database.ApiKey) *APIKey {
	apiKey := &APIKey{
		ID:        dbKey.ID,
		UserID:    dbKey.UserID,
		Name:      dbKey.Name,
		Scopes:    dbKey.Scopes,
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

func dbAPIKeyToAPIKeyWithSecret(dbKey database.ApiKey) *APIKeyWithSecret {
	return &APIKeyWithSecret{
		APIKey: *dbAPIKeyToAPIKey(dbKey),
		Key:    dbKey.ApiKey,
	}
}
