package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database/sqlc"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

// DBAuthStateStore is a database implementation of AuthStateStore.
type DBAuthStateStore struct {
	queries  *sqlc.Queries
	stateTTL time.Duration
}

// NewDBAuthStateStore creates a new database-backed auth state store.
func NewDBAuthStateStore(queries *sqlc.Queries, ttl time.Duration) *DBAuthStateStore {
	return &DBAuthStateStore{
		queries:  queries,
		stateTTL: ttl,
	}
}

// Create creates a new auth state.
func (s *DBAuthStateStore) Create(ctx context.Context, state *oidc.AuthState) error {
	return s.queries.CreateAuthState(ctx, sqlc.CreateAuthStateParams{
		State:       state.State,
		Provider:    state.ProviderName,
		RedirectUrl: state.RedirectURI,
		ExpiresAt:   state.CreatedAt.Add(s.stateTTL),
	})
}

// Get retrieves an auth state by state string.
func (s *DBAuthStateStore) Get(ctx context.Context, state string) (*oidc.AuthState, error) {
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
func (s *DBAuthStateStore) Delete(ctx context.Context, state string) error {
	return s.queries.DeleteAuthState(ctx, state)
}

// DeleteExpired deletes all expired auth states.
func (s *DBAuthStateStore) DeleteExpired(ctx context.Context) error {
	return s.queries.DeleteExpiredAuthStates(ctx)
}
