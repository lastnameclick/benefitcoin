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

// ListIdentitiesByRole returns every identity in a tenant with the given role
// (e.g. every operator/parent in a household) — used to resolve who a
// household's statement emails should go to.
func (s *Store) ListIdentitiesByRole(ctx context.Context, tenantID string, role domain.Role) ([]domain.Identity, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, customer_id, username, password_hash, role, created_at
		FROM identities WHERE tenant_id=$1 AND role=$2`, tenantID, role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Identity
	for rows.Next() {
		var i domain.Identity
		if err := rows.Scan(&i.ID, &i.TenantID, &i.CustomerID, &i.Username, &i.PasswordHash, &i.Role, &i.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// GetIdentityByCustomer finds the identity belonging to a customer (each
// customer has exactly one login) — used to resolve a notification recipient
// from an account or bounty claim, both of which reference a customer id.
func (s *Store) GetIdentityByCustomer(ctx context.Context, tenantID, customerID string) (domain.Identity, error) {
	return s.scanIdentity(ctx, `
		SELECT id, tenant_id, customer_id, username, password_hash, role, created_at
		FROM identities WHERE tenant_id=$1 AND customer_id=$2 LIMIT 1`, tenantID, customerID)
}

func (s *Store) scanIdentity(ctx context.Context, query string, args ...any) (domain.Identity, error) {
	var i domain.Identity
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&i.ID, &i.TenantID, &i.CustomerID, &i.Username, &i.PasswordHash, &i.Role, &i.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return i, ErrNotFound
	}
	return i, err
}
