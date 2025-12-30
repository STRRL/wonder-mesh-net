package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	Prefix    = "wmn_"
	KeyLength = 32
)

// Key represents a generated API key with its hash for storage.
type Key struct {
	Raw    string
	Hash   string
	Prefix string
}

// Generate creates a new API key with format "wmn_<64 hex chars>".
// Returns the raw key (show once), hash (store), and prefix (display).
func Generate() (*Key, error) {
	bytes := make([]byte, KeyLength)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}

	raw := Prefix + hex.EncodeToString(bytes)
	hash := Hash(raw)
	prefix := raw[:12] + "..."

	return &Key{
		Raw:    raw,
		Hash:   hash,
		Prefix: prefix,
	}, nil
}

// Hash computes the SHA256 hash of an API key for storage.
func Hash(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// IsAPIKey checks if a token looks like an API key (starts with prefix).
func IsAPIKey(token string) bool {
	return strings.HasPrefix(token, Prefix)
}
