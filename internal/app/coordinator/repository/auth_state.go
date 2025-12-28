package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database/sqlc"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

// DBAuthStateRepository is a database implementation of AuthStateRepository.
type DBAuthStateRepository struct {
	queries  *sqlc.Queries
	stateTTL time.Duration
}

// NewDBAuthStateRepository creates a new database-backed auth state repository.
func NewDBAuthStateRepository(queries *sqlc.Queries, ttl time.Duration) *DBAuthStateRepository {
	return &DBAuthStateRepository{
		queries:  queries,
		stateTTL: ttl,
	}
}

// Create creates a new auth state.
func (s *DBAuthStateRepository) Create(ctx context.Context, state *oidc.AuthState) error {
	return s.queries.CreateAuthState(ctx, sqlc.CreateAuthStateParams{
		State:       state.State,
		Provider:    state.ProviderName,
		RedirectUrl: state.RedirectURI,
		ExpiresAt:   state.CreatedAt.Add(s.stateTTL),
	})
}

// Get retrieves an auth state by state string.
func (s *DBAuthStateRepository) Get(ctx context.Context, state string) (*oidc.AuthState, error) {
	dbState, err := s.queries.GetAuthState(ctx, state)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if time.Now().After(dbState.ExpiresAt) {
		_ = s.queries.DeleteAuthState(ctx, state)
		return nil, nil
	}

	return &oidc.AuthState{
		State:        dbState.State,
		RedirectURI:  dbState.RedirectUrl,
		ProviderName: dbState.Provider,
		CreatedAt:    dbState.CreatedAt,
	}, nil
}

// Delete deletes an auth state.
func (s *DBAuthStateRepository) Delete(ctx context.Context, state string) error {
	return s.queries.DeleteAuthState(ctx, state)
}

// DeleteExpired deletes all expired auth states.
func (s *DBAuthStateRepository) DeleteExpired(ctx context.Context) error {
	return s.queries.DeleteExpiredAuthStates(ctx)
}
