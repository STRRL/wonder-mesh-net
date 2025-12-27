package jointoken

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims for a join token
type Claims struct {
	jwt.RegisteredClaims
	CoordinatorURL string `json:"coordinator_url"`
	HeadscaleURL   string `json:"headscale_url"`
	RealmID        string `json:"realm_id"`
	HeadscaleUser  string `json:"headscale_user"`
}

// Generator creates join tokens
type Generator struct {
	signingKey     []byte
	coordinatorURL string
	headscaleURL   string
}

// NewGenerator creates a new token generator
func NewGenerator(signingKey, coordinatorURL, headscaleURL string) *Generator {
	return &Generator{
		signingKey:     []byte(signingKey),
		coordinatorURL: coordinatorURL,
		headscaleURL:   headscaleURL,
	}
}

// Generate creates a new join token for a realm
func (g *Generator) Generate(realmID, headscaleUser string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    "wonder-mesh-net",
		},
		CoordinatorURL: g.coordinatorURL,
		HeadscaleURL:   g.headscaleURL,
		RealmID:        realmID,
		HeadscaleUser:  headscaleUser,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(g.signingKey)
}

// Validator validates join tokens
type Validator struct {
	signingKey []byte
}

// NewValidator creates a new token validator
func NewValidator(signingKey string) *Validator {
	return &Validator{
		signingKey: []byte(signingKey),
	}
}

// Validate validates a join token and returns the claims
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

// ParseUnsafe parses a token without validation (for extracting coordinator URL)
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

// EncodeForCLI encodes the token in a URL-safe format for CLI
func EncodeForCLI(token string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(token))
}

// DecodeFromCLI decodes a CLI-encoded token
func DecodeFromCLI(encoded string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	return string(data), nil
}

// JoinInfo contains the decoded join information for display
type JoinInfo struct {
	CoordinatorURL string    `json:"coordinator_url"`
	HeadscaleURL   string    `json:"headscale_url"`
	HeadscaleUser  string    `json:"headscale_user"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// GetJoinInfo extracts displayable info from a token (without full validation)
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
		HeadscaleURL:   claims.HeadscaleURL,
		HeadscaleUser:  claims.HeadscaleUser,
		ExpiresAt:      expiresAt,
	}, nil
}

// ToJSON returns the join info as JSON string
func (ji *JoinInfo) ToJSON() string {
	data, err := json.MarshalIndent(ji, "", "  ")
	if err != nil {
		slog.Error("marshal join info", "error", err)
		return `{"error": "marshal join info"}`
	}
	return string(data)
}
