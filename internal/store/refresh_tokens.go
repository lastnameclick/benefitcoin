package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// RefreshToken is a persisted refresh token (stored hashed for revocation).
type RefreshToken struct {
	ID         string
	IdentityID string
	TokenHash  string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

func (s *Store) CreateRefreshToken(ctx context.Context, t *RefreshToken) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (id, identity_id, token_hash, expires_at)
		VALUES ($1,$2,$3,$4)`, t.ID, t.IdentityID, t.TokenHash, t.ExpiresAt)
	return err
}

// GetRefreshTokenByHash returns the token row for a given hash, or ErrNotFound.
func (s *Store) GetRefreshTokenByHash(ctx context.Context, hash string) (RefreshToken, error) {
	var t RefreshToken
	err := s.pool.QueryRow(ctx, `
		SELECT id, identity_id, token_hash, expires_at, revoked_at
		FROM refresh_tokens WHERE token_hash=$1`, hash,
	).Scan(&t.ID, &t.IdentityID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

func (s *Store) RevokeRefreshToken(ctx context.Context, hash string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at=now() WHERE token_hash=$1 AND revoked_at IS NULL`, hash)
	return err
}
