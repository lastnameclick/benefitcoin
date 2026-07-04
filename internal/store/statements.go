package store

import (
	"context"
	"errors"
	"time"

	"cpal/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SaveStatement persists a generated PDF statement. Called unconditionally by
// both the on-demand download endpoint and the monthly job, so the in-app
// Inbox is always populated regardless of whether SMTP is configured.
func (s *Store) SaveStatement(ctx context.Context, tenantID, accountID string, period time.Time, pdf []byte) (string, error) {
	id := uuid.NewString()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO statements (id, tenant_id, account_id, period, pdf)
		VALUES ($1, $2, $3, $4, $5)`,
		id, tenantID, accountID, period, pdf)
	return id, err
}

// MarkStatementEmailed records that a statement was successfully emailed.
func (s *Store) MarkStatementEmailed(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE statements SET emailed_at=now() WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	return err
}

// MarkStatementViewed records the first time a holder/operator opens a
// statement from the Inbox.
func (s *Store) MarkStatementViewed(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE statements SET viewed_at=now() WHERE id=$1 AND tenant_id=$2 AND viewed_at IS NULL`, id, tenantID)
	return err
}

const statementMetaSelect = `SELECT id, tenant_id, account_id, period, generated_at, emailed_at, viewed_at FROM statements`

// ListStatements returns statement metadata (no PDF bytes) for an account,
// most recent first.
func (s *Store) ListStatements(ctx context.Context, tenantID, accountID string) ([]domain.StatementMeta, error) {
	rows, err := s.pool.Query(ctx, statementMetaSelect+`
		WHERE tenant_id=$1 AND account_id=$2 ORDER BY period DESC`, tenantID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.StatementMeta
	for rows.Next() {
		var m domain.StatementMeta
		if err := rows.Scan(&m.ID, &m.TenantID, &m.AccountID, &m.Period, &m.GeneratedAt, &m.EmailedAt, &m.ViewedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetStatementPDF fetches one statement's bytes plus its metadata (for
// Content-Disposition naming), scoped to the tenant.
func (s *Store) GetStatementPDF(ctx context.Context, tenantID, id string) (domain.StatementMeta, []byte, error) {
	var m domain.StatementMeta
	var pdf []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, account_id, period, generated_at, emailed_at, viewed_at, pdf
		FROM statements WHERE id=$1 AND tenant_id=$2`, id, tenantID,
	).Scan(&m.ID, &m.TenantID, &m.AccountID, &m.Period, &m.GeneratedAt, &m.EmailedAt, &m.ViewedAt, &pdf)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, nil, ErrNotFound
	}
	return m, pdf, err
}
