package oidc

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/database"
)

// DBAuthStateStore is a database implementation of AuthStateStore
type DBAuthStateStore struct {
	queries  *database.Queries
	stateTTL time.Duration
}

// NewDBAuthStateStore creates a new database-backed auth state store
func NewDBAuthStateStore(queries *database.Queries, ttl time.Duration) *DBAuthStateStore {
	return &DBAuthStateStore{
		queries:  queries,
		stateTTL: ttl,
	}
}

func (s *DBAuthStateStore) Create(ctx context.Context, state *AuthState) error {
	return s.queries.CreateAuthState(ctx, database.CreateAuthStateParams{
		State:        state.State,
		Nonce:        state.Nonce,
		RedirectUri:  state.RedirectURI,
		ProviderName: state.ProviderName,
		CreatedAt:    state.CreatedAt,
		ExpiresAt:    state.CreatedAt.Add(s.stateTTL),
	})
}

func (s *DBAuthStateStore) Get(ctx context.Context, state string) (*AuthState, error) {
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

	return &AuthState{
		State:        dbState.State,
		Nonce:        dbState.Nonce,
		RedirectURI:  dbState.RedirectUri,
		ProviderName: dbState.ProviderName,
		CreatedAt:    dbState.CreatedAt,
	}, nil
}

func (s *DBAuthStateStore) Delete(ctx context.Context, state string) error {
	return s.queries.DeleteAuthState(ctx, state)
}

func (s *DBAuthStateStore) DeleteExpired(ctx context.Context) error {
	return s.queries.DeleteExpiredAuthStates(ctx)
}
