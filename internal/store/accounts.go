package store

import (
	"context"
	"errors"

	"cpal/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateAccount(ctx context.Context, a *domain.Account) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO accounts (id, tenant_id, customer_id, kind, tb_account_id, currency, product, name, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE(NULLIF($9,''),'open'))
		RETURNING opened_at`,
		a.ID, a.TenantID, a.CustomerID, a.Kind, a.TBAccountID, a.Currency, a.Product, a.Name, a.Status,
	).Scan(&a.OpenedAt)
}

// CreateInternalAccount inserts an internal GL account row for a household
// (issuance or redemption). Unlike customer accounts it has no owning customer.
func (s *Store) CreateInternalAccount(ctx context.Context, a *domain.Account) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO accounts (id, tenant_id, kind, tb_account_id, currency, product, name, status)
		VALUES ($1, $2, 'internal', $3, $4, 'internal', $5, 'open')
		RETURNING opened_at`,
		a.ID, a.TenantID, a.TBAccountID, a.Currency, a.Name,
	).Scan(&a.OpenedAt)
}

func (s *Store) GetAccount(ctx context.Context, tenantID, id string) (domain.Account, error) {
	return s.scanAccount(ctx, accountSelect+` WHERE id=$1 AND tenant_id=$2`, id, tenantID)
}

func (s *Store) ListCustomerAccounts(ctx context.Context, tenantID string) ([]domain.Account, error) {
	return s.queryAccounts(ctx, accountSelect+` WHERE kind='customer' AND tenant_id=$1 ORDER BY opened_at DESC`, tenantID)
}

func (s *Store) ListAccountsByCustomer(ctx context.Context, tenantID, customerID string) ([]domain.Account, error) {
	return s.queryAccounts(ctx, accountSelect+` WHERE tenant_id=$1 AND customer_id=$2 ORDER BY opened_at DESC`, tenantID, customerID)
}

const accountSelect = `
	SELECT id, tenant_id, customer_id, kind, tb_account_id, currency, product, name, status, opened_at
	FROM accounts`

func (s *Store) scanAccount(ctx context.Context, query string, args ...any) (domain.Account, error) {
	var a domain.Account
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&a.ID, &a.TenantID, &a.CustomerID, &a.Kind, &a.TBAccountID, &a.Currency, &a.Product, &a.Name, &a.Status, &a.OpenedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *Store) queryAccounts(ctx context.Context, query string, args ...any) ([]domain.Account, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Account
	for rows.Next() {
		var a domain.Account
		if err := rows.Scan(&a.ID, &a.TenantID, &a.CustomerID, &a.Kind, &a.TBAccountID, &a.Currency,
			&a.Product, &a.Name, &a.Status, &a.OpenedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
