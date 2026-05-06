package store

import (
	"context"
	"errors"

	"cpal/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateCustomer(ctx context.Context, c *domain.Customer) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO customers (id, tenant_id, type, display_name, status)
		VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5,''),'active'))
		RETURNING created_at`,
		c.ID, c.TenantID, c.Type, c.DisplayName, c.Status,
	).Scan(&c.CreatedAt)
}

func (s *Store) GetCustomer(ctx context.Context, tenantID, id string) (domain.Customer, error) {
	var c domain.Customer
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, type, display_name, status, created_at
		FROM customers WHERE id=$1 AND tenant_id=$2`, id, tenantID,
	).Scan(&c.ID, &c.TenantID, &c.Type, &c.DisplayName, &c.Status, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Store) ListCustomers(ctx context.Context, tenantID string) ([]domain.Customer, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, type, display_name, status, created_at
		FROM customers WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Customer
	for rows.Next() {
		var c domain.Customer
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Type, &c.DisplayName, &c.Status, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
