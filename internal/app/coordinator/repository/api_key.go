package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database"
)

// APIKey represents an API key for third-party integrations.
type APIKey struct {
	ID          string
	WonderNetID string
	Name        string
	KeyHash     string
	KeyPrefix   string
	CreatedAt   time.Time
	LastUsedAt  *time.Time
	ExpiresAt   *time.Time
}

// APIKeyRepository handles API key persistence.
type APIKeyRepository struct {
	queries database.Queries
}

// NewAPIKeyRepository creates a new APIKeyRepository.
func NewAPIKeyRepository(queries database.Queries) *APIKeyRepository {
	return &APIKeyRepository{queries: queries}
}

// Create creates a new API key.
func (r *APIKeyRepository) Create(ctx context.Context, id, wonderNetID, name, keyHash, keyPrefix string, expiresAt *time.Time) (*APIKey, error) {
	var expiresAtSQL sql.NullTime
	if expiresAt != nil {
		expiresAtSQL = sql.NullTime{Time: *expiresAt, Valid: true}
	}

	row, err := r.queries.CreateAPIKey(ctx, database.CreateAPIKeyParams{
		ID:          id,
		WonderNetID: wonderNetID,
		Name:        name,
		KeyHash:     keyHash,
		KeyPrefix:   keyPrefix,
		ExpiresAt:   expiresAtSQL,
	})
	if err != nil {
		return nil, err
	}

	return apiKeyFromRow(row), nil
}

// GetByHash retrieves an API key by its hash.
func (r *APIKeyRepository) GetByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	row, err := r.queries.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return apiKeyFromRow(row), nil
}

// GetByID retrieves an API key by its ID.
func (r *APIKeyRepository) GetByID(ctx context.Context, id string) (*APIKey, error) {
	row, err := r.queries.GetAPIKeyByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return apiKeyFromRow(row), nil
}

// ListByWonderNet lists all API keys for a wonder net.
func (r *APIKeyRepository) ListByWonderNet(ctx context.Context, wonderNetID string) ([]*APIKey, error) {
	rows, err := r.queries.ListAPIKeysByWonderNet(ctx, wonderNetID)
	if err != nil {
		return nil, err
	}

	keys := make([]*APIKey, len(rows))
	for i, row := range rows {
		keys[i] = apiKeyFromRow(row)
	}
	return keys, nil
}

// Delete deletes an API key by ID.
func (r *APIKeyRepository) Delete(ctx context.Context, id string) error {
	return r.queries.DeleteAPIKey(ctx, id)
}

// UpdateLastUsed updates the last_used_at timestamp.
func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, id string) error {
	return r.queries.UpdateAPIKeyLastUsed(ctx, id)
}

func apiKeyFromRow(row database.APIKey) *APIKey {
	key := &APIKey{
		ID:          row.ID,
		WonderNetID: row.WonderNetID,
		Name:        row.Name,
		KeyHash:     row.KeyHash,
		KeyPrefix:   row.KeyPrefix,
		CreatedAt:   row.CreatedAt,
	}
	if row.LastUsedAt.Valid {
		key.LastUsedAt = &row.LastUsedAt.Time
	}
	if row.ExpiresAt.Valid {
		key.ExpiresAt = &row.ExpiresAt.Time
	}
	return key
}
