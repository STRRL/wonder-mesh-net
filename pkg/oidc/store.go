package oidc

import (
	"context"
	"sync"
	"time"
)

// AuthStateStore defines the interface for persisting OIDC authentication states.
// Implementations must be safe for concurrent use from multiple goroutines.
// Auth states are short-lived and should be automatically expired based on a TTL.
type AuthStateStore interface {
	// Create stores a new auth state. The state's State field is used as the key.
	Create(ctx context.Context, state *AuthState) error

	// Get retrieves an auth state by its state string.
	// Returns nil, nil if the state does not exist.
	Get(ctx context.Context, state string) (*AuthState, error)

	// Delete removes an auth state by its state string.
	// Returns nil if the state does not exist.
	Delete(ctx context.Context, state string) error

	// DeleteExpired removes all expired auth states from the store.
	DeleteExpired(ctx context.Context) error
}

// MemoryAuthStateStore is an in-memory implementation of AuthStateStore.
// It is suitable for single-instance deployments but will lose state on restart.
// For multi-instance deployments, use a database-backed implementation.
type MemoryAuthStateStore struct {
	mu       sync.RWMutex
	states   map[string]*AuthState
	stateTTL time.Duration
}

// NewMemoryAuthStateStore creates a new in-memory auth state store with the given TTL.
// The TTL is used by DeleteExpired to determine which states should be removed.
func NewMemoryAuthStateStore(ttl time.Duration) *MemoryAuthStateStore {
	return &MemoryAuthStateStore{
		states:   make(map[string]*AuthState),
		stateTTL: ttl,
	}
}

// Create stores the auth state in memory using its State field as the key.
func (s *MemoryAuthStateStore) Create(ctx context.Context, state *AuthState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state.State] = state
	return nil
}

// Get retrieves an auth state by its state string.
// Returns nil, nil if the state does not exist.
func (s *MemoryAuthStateStore) Get(ctx context.Context, state string) (*AuthState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	authState, ok := s.states[state]
	if !ok {
		return nil, nil
	}
	return authState, nil
}

// Delete removes an auth state from memory.
func (s *MemoryAuthStateStore) Delete(ctx context.Context, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, state)
	return nil
}

// DeleteExpired removes all states older than the configured TTL.
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
