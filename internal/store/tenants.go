package store

import (
	"context"
	"errors"

	"cpal/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateTenant(ctx context.Context, t *domain.Tenant) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO tenants
			(id, name, status, issuance_account_id, redemption_account_id, issuance_tb_id, redemption_tb_id)
		VALUES ($1, $2, COALESCE(NULLIF($3,''),'active'), $4, $5, $6, $7)
		RETURNING created_at`,
		t.ID, t.Name, t.Status, t.IssuanceAccountID, t.RedemptionAccountID, t.IssuanceTBID, t.RedemptionTBID,
	).Scan(&t.CreatedAt)
}

func (s *Store) GetTenant(ctx context.Context, id string) (domain.Tenant, error) {
	var t domain.Tenant
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, status, issuance_account_id, redemption_account_id,
		       issuance_tb_id, redemption_tb_id, created_at
		FROM tenants WHERE id=$1`, id,
	).Scan(&t.ID, &t.Name, &t.Status, &t.IssuanceAccountID, &t.RedemptionAccountID,
		&t.IssuanceTBID, &t.RedemptionTBID, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// ListActiveTenants returns every active, fully-onboarded household (one
// whose GL accounts were wired up at signup), for the monthly statement job
// and the notification sweep to iterate. Excludes the legacy default
// household migration 0003 seeds for pre-multi-tenant data — on a fresh
// database it never gets real GL accounts, so its issuance/redemption
// account ids stay NULL.
func (s *Store) ListActiveTenants(ctx context.Context) ([]domain.Tenant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, status, issuance_account_id, redemption_account_id,
		       issuance_tb_id, redemption_tb_id, created_at
		FROM tenants
		WHERE status='active' AND issuance_account_id IS NOT NULL AND redemption_account_id IS NOT NULL
		ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &t.IssuanceAccountID, &t.RedemptionAccountID,
			&t.IssuanceTBID, &t.RedemptionTBID, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
