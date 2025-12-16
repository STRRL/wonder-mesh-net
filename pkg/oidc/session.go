package oidc

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/database"
)

// Session represents a user session
type Session struct {
	ID         string
	UserID     string
	Issuer     string
	Subject    string
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	LastUsedAt time.Time
}

// SessionStore is the interface for storing sessions
type SessionStore interface {
	Create(ctx context.Context, session *Session) error
	Get(ctx context.Context, id string) (*Session, error)
	UpdateLastUsed(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context) error
	DeleteUserSessions(ctx context.Context, userID string) error
}

// DBSessionStore is a database implementation of SessionStore
type DBSessionStore struct {
	queries *database.Queries
}

// NewDBSessionStore creates a new database-backed session store
func NewDBSessionStore(queries *database.Queries) *DBSessionStore {
	return &DBSessionStore{queries: queries}
}

// GenerateSessionID generates a random session ID
func GenerateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *DBSessionStore) Create(ctx context.Context, session *Session) error {
	var expiresAt sql.NullTime
	if session.ExpiresAt != nil {
		expiresAt = sql.NullTime{Time: *session.ExpiresAt, Valid: true}
	}

	return s.queries.CreateSession(ctx, database.CreateSessionParams{
		ID:         session.ID,
		UserID:     session.UserID,
		Issuer:     session.Issuer,
		Subject:    session.Subject,
		CreatedAt:  session.CreatedAt,
		ExpiresAt:  expiresAt,
		LastUsedAt: session.LastUsedAt,
	})
}

func (s *DBSessionStore) Get(ctx context.Context, id string) (*Session, error) {
	dbSession, err := s.queries.GetSession(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if dbSession.ExpiresAt.Valid && time.Now().After(dbSession.ExpiresAt.Time) {
		_ = s.queries.DeleteSession(ctx, id)
		return nil, nil
	}

	session := &Session{
		ID:         dbSession.ID,
		UserID:     dbSession.UserID,
		Issuer:     dbSession.Issuer,
		Subject:    dbSession.Subject,
		CreatedAt:  dbSession.CreatedAt,
		LastUsedAt: dbSession.LastUsedAt,
	}

	if dbSession.ExpiresAt.Valid {
		session.ExpiresAt = &dbSession.ExpiresAt.Time
	}

	return session, nil
}

func (s *DBSessionStore) UpdateLastUsed(ctx context.Context, id string) error {
	return s.queries.UpdateSessionLastUsed(ctx, database.UpdateSessionLastUsedParams{
		LastUsedAt: time.Now(),
		ID:         id,
	})
}

func (s *DBSessionStore) Delete(ctx context.Context, id string) error {
	return s.queries.DeleteSession(ctx, id)
}

func (s *DBSessionStore) DeleteExpired(ctx context.Context) error {
	return s.queries.DeleteExpiredSessions(ctx)
}

func (s *DBSessionStore) DeleteUserSessions(ctx context.Context, userID string) error {
	return s.queries.DeleteUserSessions(ctx, userID)
}
