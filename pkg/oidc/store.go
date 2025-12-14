package oidc

import (
	"context"
	"sync"
	"time"
)

// AuthStateStore is the interface for storing OIDC auth states
type AuthStateStore interface {
	Create(ctx context.Context, state *AuthState) error
	Get(ctx context.Context, state string) (*AuthState, error)
	Delete(ctx context.Context, state string) error
	DeleteExpired(ctx context.Context) error
}

// MemoryAuthStateStore is an in-memory implementation of AuthStateStore
type MemoryAuthStateStore struct {
	mu       sync.RWMutex
	states   map[string]*AuthState
	stateTTL time.Duration
}

// NewMemoryAuthStateStore creates a new in-memory auth state store
func NewMemoryAuthStateStore(ttl time.Duration) *MemoryAuthStateStore {
	return &MemoryAuthStateStore{
		states:   make(map[string]*AuthState),
		stateTTL: ttl,
	}
}

func (s *MemoryAuthStateStore) Create(ctx context.Context, state *AuthState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state.State] = state
	return nil
}

func (s *MemoryAuthStateStore) Get(ctx context.Context, state string) (*AuthState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	authState, ok := s.states[state]
	if !ok {
		return nil, nil
	}
	return authState, nil
}

func (s *MemoryAuthStateStore) Delete(ctx context.Context, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, state)
	return nil
}

func (s *MemoryAuthStateStore) DeleteExpired(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for state, authState := range s.states {
		if now.Sub(authState.CreatedAt) > s.stateTTL {
			delete(s.states, state)
		}
	}
	return nil
}
