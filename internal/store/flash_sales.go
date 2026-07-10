package store

import (
	"context"
	"errors"

	"cpal/internal/domain"

	"github.com/jackc/pgx/v5"
)

// CreateFlashSale inserts a new flash sale, rejecting it with ErrConflict if
// its window overlaps an existing non-canceled sale for the tenant — this
// keeps "the currently active sale" always unambiguous.
func (s *Store) CreateFlashSale(ctx context.Context, fs *domain.FlashSale) error {
	var conflict bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM flash_sales
			WHERE tenant_id = $1 AND canceled_at IS NULL
			  AND starts_at < $2 AND ends_at > $3
		)`,
		fs.TenantID, fs.EndsAt, fs.StartsAt,
	).Scan(&conflict)
	if err != nil {
		return err
	}
	if conflict {
		return ErrConflict
	}

	return s.pool.QueryRow(ctx, `
		INSERT INTO flash_sales (id, tenant_id, discount_type, percent_off, amount_off_minor, starts_at, ends_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`,
		fs.ID, fs.TenantID, fs.DiscountType, fs.PercentOff, fs.AmountOffMinor, fs.StartsAt, fs.EndsAt, fs.CreatedBy,
	).Scan(&fs.CreatedAt)
}

const flashSaleSelect = `SELECT id, tenant_id, discount_type, percent_off, amount_off_minor, starts_at, ends_at, canceled_at, created_by, created_at FROM flash_sales`

func scanFlashSale(row pgx.Row) (domain.FlashSale, error) {
	var fs domain.FlashSale
	err := row.Scan(&fs.ID, &fs.TenantID, &fs.DiscountType, &fs.PercentOff, &fs.AmountOffMinor,
		&fs.StartsAt, &fs.EndsAt, &fs.CanceledAt, &fs.CreatedBy, &fs.CreatedAt)
	return fs, err
}

// ListFlashSales returns every flash sale ever scheduled for the tenant,
// most recently starting first — the operator history/schedule view.
func (s *Store) ListFlashSales(ctx context.Context, tenantID string) ([]domain.FlashSale, error) {
	rows, err := s.pool.Query(ctx, flashSaleSelect+` WHERE tenant_id=$1 ORDER BY starts_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.FlashSale
	for rows.Next() {
		fs, err := scanFlashSale(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, fs)
	}
	return out, rows.Err()
}

// GetActiveFlashSale returns the sale currently in effect for the tenant, if
// any. The no-overlap invariant enforced at creation means at most one row
// can ever match.
func (s *Store) GetActiveFlashSale(ctx context.Context, tenantID string) (domain.FlashSale, error) {
	fs, err := scanFlashSale(s.pool.QueryRow(ctx, flashSaleSelect+`
		WHERE tenant_id=$1 AND canceled_at IS NULL AND starts_at <= now() AND ends_at > now()
		ORDER BY starts_at DESC LIMIT 1`, tenantID))
	if errors.Is(err, pgx.ErrNoRows) {
		return fs, ErrNotFound
	}
	return fs, err
}

// CancelFlashSale ends a scheduled or active sale early. Returns ErrNotFound
// if it doesn't exist, already ended, or was already canceled.
func (s *Store) CancelFlashSale(ctx context.Context, tenantID, id string) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE flash_sales SET canceled_at = now()
		WHERE id=$1 AND tenant_id=$2 AND canceled_at IS NULL AND ends_at > now()`,
		id, tenantID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListFlashSalesStartingNow returns active sales that haven't yet had their
// "flash sale started" notification sent, so the periodic sweep can alert
// holders about sales that were scheduled ahead of time.
func (s *Store) ListFlashSalesStartingNow(ctx context.Context, tenantID string) ([]domain.FlashSale, error) {
	rows, err := s.pool.Query(ctx, flashSaleSelect+`
		WHERE tenant_id=$1 AND canceled_at IS NULL AND started_notified_at IS NULL
		  AND starts_at <= now() AND ends_at > now()`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.FlashSale
	for rows.Next() {
		fs, err := scanFlashSale(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, fs)
	}
	return out, rows.Err()
}

// MarkFlashSaleStartNotified flags a flash sale as having already sent its
// "started" alert, so the sweep doesn't repeat it on every tick.
func (s *Store) MarkFlashSaleStartNotified(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE flash_sales SET started_notified_at = now() WHERE id=$1 AND tenant_id=$2`,
		id, tenantID)
	return err
}
