package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database/sqlc"
)

// Session represents a user session.
type Session struct {
	ID         string
	UserID     string
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	LastUsedAt time.Time
}

// SessionStore is the interface for storing sessions.
type SessionStore interface {
	Create(ctx context.Context, session *Session) error
	Get(ctx context.Context, id string) (*Session, error)
	UpdateLastUsed(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context) error
	DeleteUserSessions(ctx context.Context, userID string) error
}

// DBSessionStore is a database implementation of SessionStore.
type DBSessionStore struct {
	queries *sqlc.Queries
}

// NewDBSessionStore creates a new database-backed session store.
func NewDBSessionStore(queries *sqlc.Queries) *DBSessionStore {
	return &DBSessionStore{queries: queries}
}

// GenerateSessionID generates a random session ID.
func GenerateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Create creates a new session.
func (s *DBSessionStore) Create(ctx context.Context, session *Session) error {
	var expiresAt sql.NullTime
	if session.ExpiresAt != nil {
		expiresAt = sql.NullTime{Time: *session.ExpiresAt, Valid: true}
	}

	return s.queries.CreateSession(ctx, sqlc.CreateSessionParams{
		ID:        session.ID,
		UserID:    session.UserID,
		ExpiresAt: expiresAt,
	})
}

// Get retrieves a session by ID.
func (s *DBSessionStore) Get(ctx context.Context, id string) (*Session, error) {
	row, err := s.queries.GetSession(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if row.ExpiresAt.Valid && time.Now().After(row.ExpiresAt.Time) {
		_ = s.queries.DeleteSession(ctx, id)
		return nil, nil
	}

	session := &Session{
		ID:         row.ID,
		UserID:     row.UserID,
		CreatedAt:  row.CreatedAt,
		LastUsedAt: row.LastUsedAt,
	}

	if row.ExpiresAt.Valid {
		session.ExpiresAt = &row.ExpiresAt.Time
	}

	return session, nil
}

// UpdateLastUsed updates the last used timestamp.
func (s *DBSessionStore) UpdateLastUsed(ctx context.Context, id string) error {
	return s.queries.UpdateSessionLastUsed(ctx, id)
}

// Delete deletes a session.
func (s *DBSessionStore) Delete(ctx context.Context, id string) error {
	return s.queries.DeleteSession(ctx, id)
}

// DeleteExpired deletes all expired sessions.
func (s *DBSessionStore) DeleteExpired(ctx context.Context) error {
	return s.queries.DeleteExpiredSessions(ctx)
}

// DeleteUserSessions deletes all sessions for a user.
func (s *DBSessionStore) DeleteUserSessions(ctx context.Context, userID string) error {
	return s.queries.DeleteUserSessions(ctx, userID)
}
