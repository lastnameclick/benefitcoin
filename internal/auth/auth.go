// Package auth provides password hashing, JWT access tokens, opaque refresh
// tokens, and RBAC middleware.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"cpal/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Claims are the authenticated principal carried in an access token.
type Claims struct {
	IdentityID string      `json:"sub"`
	TenantID   string      `json:"tid"`
	CustomerID string      `json:"cid"`
	Username   string      `json:"usr"`
	Role       domain.Role `json:"role"`
}

// Manager issues and verifies tokens.
type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewManager(secret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (m *Manager) RefreshTTL() time.Duration { return m.refreshTTL }
func (m *Manager) AccessTTL() time.Duration  { return m.accessTTL }

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether pw matches the stored bcrypt hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

type jwtClaims struct {
	TenantID   string      `json:"tid"`
	CustomerID string      `json:"cid"`
	Username   string      `json:"usr"`
	Role       domain.Role `json:"role"`
	jwt.RegisteredClaims
}

// IssueAccess mints a signed JWT access token and returns it with its expiry.
func (m *Manager) IssueAccess(c Claims, now time.Time) (string, time.Time, error) {
	exp := now.Add(m.accessTTL)
	claims := jwtClaims{
		TenantID:   c.TenantID,
		CustomerID: c.CustomerID,
		Username:   c.Username,
		Role:       c.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   c.IdentityID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(m.secret)
	return signed, exp, err
}

// ParseAccess validates a token and returns its claims.
func (m *Manager) ParseAccess(token string) (Claims, error) {
	var jc jwtClaims
	_, err := jwt.ParseWithClaims(token, &jc, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return Claims{}, err
	}
	return Claims{
		IdentityID: jc.Subject,
		TenantID:   jc.TenantID,
		CustomerID: jc.CustomerID,
		Username:   jc.Username,
		Role:       jc.Role,
	}, nil
}

// NewRefreshToken generates a random opaque refresh token, returning the raw
// value (given to the client) and its hash (stored for revocation/lookup).
func NewRefreshToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	return raw, HashRefresh(raw), nil
}

// HashRefresh hashes a raw refresh token for storage/lookup.
func HashRefresh(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
