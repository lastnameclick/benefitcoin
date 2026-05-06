package store

import (
	"context"
	"errors"

	"cpal/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateIdentity(ctx context.Context, i *domain.Identity) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO identities (id, tenant_id, customer_id, username, password_hash, role)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`,
		i.ID, i.TenantID, i.CustomerID, i.Username, i.PasswordHash, i.Role,
	).Scan(&i.CreatedAt)
}

// GetIdentityByUsername looks up a login across all tenants — usernames are
// globally unique, so login needs no household selector.
func (s *Store) GetIdentityByUsername(ctx context.Context, username string) (domain.Identity, error) {
	return s.scanIdentity(ctx, `
		SELECT id, tenant_id, customer_id, username, password_hash, role, created_at
		FROM identities WHERE username=$1`, username)
}

func (s *Store) GetIdentity(ctx context.Context, id string) (domain.Identity, error) {
	return s.scanIdentity(ctx, `
		SELECT id, tenant_id, customer_id, username, password_hash, role, created_at
		FROM identities WHERE id=$1`, id)
}

func (s *Store) scanIdentity(ctx context.Context, query string, arg any) (domain.Identity, error) {
	var i domain.Identity
	err := s.pool.QueryRow(ctx, query, arg).Scan(
		&i.ID, &i.TenantID, &i.CustomerID, &i.Username, &i.PasswordHash, &i.Role, &i.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return i, ErrNotFound
	}
	return i, err
}
