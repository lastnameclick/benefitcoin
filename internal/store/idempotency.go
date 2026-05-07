package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// IdempotencyRecord is a stored response for a previously-seen idempotency key.
type IdempotencyRecord struct {
	IdentityID   string
	Key          string
	Method       string
	Path         string
	RequestHash  string
	StatusCode   int
	ResponseBody []byte
}

// GetIdempotency returns a stored record for (identityID, key), or ErrNotFound.
func (s *Store) GetIdempotency(ctx context.Context, identityID, key string) (IdempotencyRecord, error) {
	var r IdempotencyRecord
	err := s.pool.QueryRow(ctx, `
		SELECT identity_id, key, method, path, request_hash, status_code, response_body
		FROM idempotency_keys WHERE identity_id=$1 AND key=$2`, identityID, key,
	).Scan(&r.IdentityID, &r.Key, &r.Method, &r.Path, &r.RequestHash, &r.StatusCode, &r.ResponseBody)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// SaveIdempotency stores a response. Conflicts (concurrent identical request)
// are ignored so the first writer wins.
func (s *Store) SaveIdempotency(ctx context.Context, r IdempotencyRecord) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO idempotency_keys
			(identity_id, key, method, path, request_hash, status_code, response_body)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (identity_id, key) DO NOTHING`,
		r.IdentityID, r.Key, r.Method, r.Path, r.RequestHash, r.StatusCode, r.ResponseBody)
	return err
}
