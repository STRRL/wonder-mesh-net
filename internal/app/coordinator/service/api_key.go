package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/apikey"
)

var (
	ErrAPIKeyNotFound = errors.New("api key not found")
	ErrAPIKeyExpired  = errors.New("api key expired")
)

// APIKeyDetails contains the details of a newly created API key.
// The raw key is only available at creation time.
type APIKeyDetails struct {
	ID        string
	Name      string
	Key       string
	KeyPrefix string
	ExpiresAt *time.Time
}

// APIKeyInfo contains information about an existing API key (no raw key).
type APIKeyInfo struct {
	ID         string
	Name       string
	KeyPrefix  string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
}

// APIKeyService manages API keys for third-party integrations.
type APIKeyService struct {
	apiKeyRepository    *repository.APIKeyRepository
	wonderNetRepository *repository.WonderNetRepository
}

// NewAPIKeyService creates a new APIKeyService.
func NewAPIKeyService(
	apiKeyRepository *repository.APIKeyRepository,
	wonderNetRepository *repository.WonderNetRepository,
) *APIKeyService {
	return &APIKeyService{
		apiKeyRepository:    apiKeyRepository,
		wonderNetRepository: wonderNetRepository,
	}
}

// CreateAPIKey creates a new API key for a wonder net.
func (s *APIKeyService) CreateAPIKey(ctx context.Context, wonderNetID, name string, expiresAt *time.Time) (*APIKeyDetails, error) {
	key, err := apikey.Generate()
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
	_, err = s.apiKeyRepository.Create(ctx, id, wonderNetID, name, key.Hash, key.Prefix, expiresAt)
	if err != nil {
		return nil, err
	}

	slog.Info("created api key", "id", id, "wonder_net_id", wonderNetID, "name", name)

	return &APIKeyDetails{
		ID:        id,
		Name:      name,
		Key:       key.Raw,
		KeyPrefix: key.Prefix,
		ExpiresAt: expiresAt,
	}, nil
}

// ListAPIKeys lists all API keys for a wonder net (without raw keys).
func (s *APIKeyService) ListAPIKeys(ctx context.Context, wonderNetID string) ([]*APIKeyInfo, error) {
	keys, err := s.apiKeyRepository.ListByWonderNet(ctx, wonderNetID)
	if err != nil {
		return nil, err
	}

	infos := make([]*APIKeyInfo, len(keys))
	for i, key := range keys {
		infos[i] = &APIKeyInfo{
			ID:         key.ID,
			Name:       key.Name,
			KeyPrefix:  key.KeyPrefix,
			CreatedAt:  key.CreatedAt,
			LastUsedAt: key.LastUsedAt,
			ExpiresAt:  key.ExpiresAt,
		}
	}
	return infos, nil
}

// DeleteAPIKey deletes an API key.
func (s *APIKeyService) DeleteAPIKey(ctx context.Context, wonderNetID, keyID string) error {
	key, err := s.apiKeyRepository.GetByID(ctx, keyID)
	if err != nil {
		return err
	}
	if key == nil {
		return ErrAPIKeyNotFound
	}
	if key.WonderNetID != wonderNetID {
		return ErrAPIKeyNotFound
	}

	if err := s.apiKeyRepository.Delete(ctx, keyID); err != nil {
		return err
	}

	slog.Info("deleted api key", "id", keyID, "wonder_net_id", wonderNetID)
	return nil
}

// ValidateAPIKey validates an API key and returns the associated wonder net.
func (s *APIKeyService) ValidateAPIKey(ctx context.Context, rawKey string) (*repository.WonderNet, error) {
	keyHash := apikey.Hash(rawKey)
	key, err := s.apiKeyRepository.GetByHash(ctx, keyHash)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, ErrAPIKeyNotFound
	}

	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return nil, ErrAPIKeyExpired
	}

	go func() {
		if err := s.apiKeyRepository.UpdateLastUsed(context.Background(), key.ID); err != nil {
			slog.Warn("update api key last used", "error", err, "id", key.ID)
		}
	}()

	wonderNet, err := s.wonderNetRepository.Get(ctx, key.WonderNetID)
	if err != nil {
		return nil, err
	}
	if wonderNet == nil {
		return nil, ErrNoWonderNet
	}

	return wonderNet, nil
}
