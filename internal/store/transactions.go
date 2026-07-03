package store

import (
	"context"
	"errors"
	"time"

	"cpal/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateTransaction(ctx context.Context, t *domain.Transaction) error {
	details := t.Details
	if details == nil {
		details = map[string]any{}
	}
	return s.pool.QueryRow(ctx, `
		INSERT INTO transactions
			(id, tenant_id, type, status, account_id, gl_account_id, amount_minor, task_id, memo,
			 tb_pending_transfer_id, tb_post_transfer_id, effective_at, details,
			 created_by, decided_by, decided_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING created_at`,
		t.ID, t.TenantID, t.Type, t.Status, t.AccountID, t.GLAccountID, t.AmountMinor, t.TaskID, t.Memo,
		t.TBPendingTransferID, t.TBPostTransferID, t.EffectiveAt, details,
		t.CreatedBy, t.DecidedBy, t.DecidedAt,
	).Scan(&t.CreatedAt)
}

// DecideTransaction records the outcome of an authorization hold (settle/void),
// only if it is still pending and within the tenant. Returns ErrNotFound if the
// row is missing or no longer pending (prevents double-decisions).
func (s *Store) DecideTransaction(ctx context.Context, tenantID, id string, status domain.TxStatus, postTransferID *string, decidedBy string, decidedAt time.Time) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE transactions
		SET status=$3, tb_post_transfer_id=$4, decided_by=$5, decided_at=$6
		WHERE id=$1 AND tenant_id=$2 AND status='pending'`,
		id, tenantID, status, postTransferID, decidedBy, decidedAt)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AdjustTransactionAmount changes the amount and backing ledger hold of a
// still-pending transaction (an operator revising a proposed reward before
// deciding it). Returns ErrNotFound if it's missing or no longer pending.
func (s *Store) AdjustTransactionAmount(ctx context.Context, tenantID, id string, amountMinor int64, pendingTransferID string) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE transactions
		SET amount_minor=$3, tb_pending_transfer_id=$4
		WHERE id=$1 AND tenant_id=$2 AND status='pending'`,
		id, tenantID, amountMinor, pendingTransferID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetTransaction(ctx context.Context, tenantID, id string) (domain.Transaction, error) {
	return s.scanTx(ctx, txSelect+` WHERE id=$1 AND tenant_id=$2`, id, tenantID)
}

// ListTransactions filters by tenant plus optional status and/or account id
// (empty = no filter).
func (s *Store) ListTransactions(ctx context.Context, tenantID, status, accountID string, limit int) ([]domain.Transaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query := txSelect + ` WHERE tenant_id=$1 AND ($2='' OR status=$2) AND ($3='' OR account_id=$3::uuid)
		ORDER BY created_at DESC LIMIT $4`
	rows, err := s.pool.Query(ctx, query, tenantID, status, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Transaction
	for rows.Next() {
		t, err := scanTxRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListTransactionsInRange returns an account's settled transactions whose
// value date falls in [from, to), oldest first — the statement's line items.
func (s *Store) ListTransactionsInRange(ctx context.Context, tenantID, accountID string, from, to time.Time) ([]domain.Transaction, error) {
	rows, err := s.pool.Query(ctx, txSelect+`
		WHERE tenant_id=$1 AND account_id=$2 AND status='settled'
		  AND COALESCE(effective_at, created_at) >= $3 AND COALESCE(effective_at, created_at) < $4
		ORDER BY COALESCE(effective_at, created_at)`,
		tenantID, accountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Transaction
	for rows.Next() {
		t, err := scanTxRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

const txSelect = `
	SELECT id, tenant_id, type, status, account_id, gl_account_id, amount_minor, task_id, memo,
	       tb_pending_transfer_id, tb_post_transfer_id, effective_at, details,
	       created_by, created_at, decided_by, decided_at
	FROM transactions`

func (s *Store) scanTx(ctx context.Context, query string, args ...any) (domain.Transaction, error) {
	row := s.pool.QueryRow(ctx, query, args...)
	t, err := scanTxRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanTxRow(row scannable) (domain.Transaction, error) {
	var t domain.Transaction
	err := row.Scan(&t.ID, &t.TenantID, &t.Type, &t.Status, &t.AccountID, &t.GLAccountID, &t.AmountMinor,
		&t.TaskID, &t.Memo, &t.TBPendingTransferID, &t.TBPostTransferID, &t.EffectiveAt, &t.Details,
		&t.CreatedBy, &t.CreatedAt, &t.DecidedBy, &t.DecidedAt)
	return t, err
}
