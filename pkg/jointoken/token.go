// Package jointoken provides JWT-based join tokens for worker nodes to securely
// join a Wonder Mesh Net wonder net.
//
// Join tokens are short-lived JWTs that encode the minimal information for a
// worker node to contact the coordinator and obtain mesh credentials. The token
// does not contain mesh-specific information (like Headscale URLs); that info
// is negotiated during the join HTTP exchange.
//
// The token flow is:
//
//  1. User authenticates via Keycloak and requests a join token from the coordinator
//  2. Coordinator generates a signed JWT containing coordinator URL and wonder net ID
//  3. User transfers the token to the worker node (via CLI copy-paste or file)
//  4. Worker contacts coordinator to exchange the token for mesh credentials
//  5. Coordinator returns mesh_type and metadata (e.g., Tailscale login_server, authkey)
//  6. Worker uses the mesh-specific credentials to join the network
//
// Tokens are signed using HMAC-SHA256 with a shared secret between coordinator
// instances. The token TTL is typically short (hours) to limit exposure if leaked.
package jointoken

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims for a join token.
//
// It embeds the standard JWT registered claims (iat, exp, iss) and adds
// the minimal claims needed for a worker to contact the coordinator.
// Mesh-specific information (like Headscale URLs) is NOT included here;
// it's returned in the HTTP response when the token is exchanged.
type Claims struct {
	jwt.RegisteredClaims

	// CoordinatorURL is the URL of the coordinator API that the worker
	// should contact to exchange this token for mesh credentials.
	CoordinatorURL string `json:"coordinator_url"`

	// WonderNetID is the unique identifier for the wonder net (tenant namespace)
	// that this worker will join. Used for multi-tenant isolation.
	WonderNetID string `json:"wonder_net_id"`
}

// Generator creates signed join tokens for worker nodes.
//
// It holds the signing key and coordinator URL that are embedded into every token.
// A single Generator instance should be reused across requests since it's
// safe for concurrent use.
type Generator struct {
	signingKey     []byte
	coordinatorURL string
}

// NewGenerator creates a new token generator with the given configuration.
//
// Parameters:
//   - signingKey: The HMAC-SHA256 secret key for signing tokens. Must be shared
//     with all coordinator instances and the Validator.
//   - coordinatorURL: The public URL of the coordinator API (e.g., "https://wonder.example.com").
//
// The signingKey should be at least 32 bytes of random data for security.
// Use "openssl rand -hex 32" to generate a suitable key.
func NewGenerator(signingKey, coordinatorURL string) *Generator {
	return &Generator{
		signingKey:     []byte(signingKey),
		coordinatorURL: coordinatorURL,
	}
}

// Generate creates a new signed join token for the specified wonder net.
//
// Parameters:
//   - wonderNetID: The unique identifier for the wonder net (UUID format).
//   - ttl: How long the token should be valid. Typical values are 1-24 hours.
//
// Returns the signed JWT string, or an error if signing fails.
//
// The generated token includes:
//   - Standard JWT claims: iat (issued at), exp (expiration), iss (issuer)
//   - Custom claims: coordinator URL, wonder net ID
func (g *Generator) Generate(wonderNetID string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    "wonder-mesh-net",
		},
		CoordinatorURL: g.coordinatorURL,
		WonderNetID:    wonderNetID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(g.signingKey)
}

// Validator validates join tokens and extracts their claims.
//
// It verifies both the signature and expiration of tokens. A single Validator
// instance should be reused across requests since it's safe for concurrent use.
type Validator struct {
	signingKey []byte
}

// NewValidator creates a new token validator with the given signing key.
//
// The signingKey must match the key used by the Generator that created the tokens.
func NewValidator(signingKey string) *Validator {
	return &Validator{
		signingKey: []byte(signingKey),
	}
}

// Validate verifies a join token's signature and expiration, returning the claims.
//
// This method performs full validation:
//   - Verifies the HMAC-SHA256 signature matches
//   - Checks that the token has not expired
//   - Ensures the signing method is HMAC (prevents algorithm confusion attacks)
//
// Returns the decoded Claims on success, or an error if validation fails.
// Common error cases include: expired token, invalid signature, malformed token.
func (v *Validator) Validate(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.signingKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// ParseUnsafe parses a token without validating the signature or expiration.
//
// This is used by the worker CLI to extract the coordinator URL from a token
// before contacting the coordinator to validate it. Since the worker doesn't
// have the signing key, it cannot validate tokens locally.
//
// WARNING: Do not trust claims from this function for authorization decisions.
// Always use Validator.Validate for server-side token validation.
//
// Returns the decoded Claims, or an error if the token is malformed.
func ParseUnsafe(tokenString string) (*Claims, error) {
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// EncodeForCLI encodes a JWT token in a URL-safe base64 format for CLI usage.
//
// This encoding makes tokens easier to copy-paste in terminal environments by:
//   - Using URL-safe base64 (no + or / characters)
//   - Omitting padding (no = characters)
//
// Use DecodeFromCLI to reverse this encoding.
func EncodeForCLI(token string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(token))
}

// DecodeFromCLI decodes a CLI-encoded token back to its original JWT format.
//
// Returns the original JWT string, or an error if the input is not valid
// URL-safe base64 encoding.
func DecodeFromCLI(encoded string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	return string(data), nil
}

// JoinInfo contains a subset of token claims formatted for user display.
//
// This struct is used by the CLI to show users what a join token contains
// before they use it to join a mesh.
type JoinInfo struct {
	// CoordinatorURL is the URL the worker will contact to exchange the token.
	CoordinatorURL string `json:"coordinator_url"`

	// WonderNetID is the wonder net the worker will join.
	WonderNetID string `json:"wonder_net_id"`

	// ExpiresAt is when the token becomes invalid.
	ExpiresAt time.Time `json:"expires_at"`
}

// GetJoinInfo extracts displayable information from a token.
//
// This uses ParseUnsafe internally, so it does not validate the token's
// signature or expiration. It's intended for CLI display purposes only.
//
// Returns the extracted JoinInfo, or an error if the token is malformed.
func GetJoinInfo(tokenString string) (*JoinInfo, error) {
	claims, err := ParseUnsafe(tokenString)
	if err != nil {
		return nil, err
	}

	var expiresAt time.Time
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}

	return &JoinInfo{
		CoordinatorURL: claims.CoordinatorURL,
		WonderNetID:    claims.WonderNetID,
		ExpiresAt:      expiresAt,
	}, nil
}

// ToJSON returns the JoinInfo as a pretty-printed JSON string.
//
// This is used by the CLI for --json output format. On serialization error,
// it logs the error and returns a JSON object with an error field.
func (ji *JoinInfo) ToJSON() string {
	data, err := json.MarshalIndent(ji, "", "  ")
	if err != nil {
		slog.Error("marshal join info", "error", err)
		return `{"error": "marshal join info"}`
	}
	return string(data)
}
