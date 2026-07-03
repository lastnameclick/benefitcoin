package store

import (
	"context"
	"errors"

	"cpal/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateTask(ctx context.Context, t *domain.Task) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO tasks (id, tenant_id, name, description, value_minor, active, is_bounty)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`,
		t.ID, t.TenantID, t.Name, t.Description, t.ValueMinor, t.Active, t.IsBounty,
	).Scan(&t.CreatedAt)
}

const taskSelect = `SELECT id, tenant_id, name, description, value_minor, active, is_bounty, claimed_by, claimed_at, created_at FROM tasks`

func (s *Store) GetTask(ctx context.Context, tenantID, id string) (domain.Task, error) {
	var t domain.Task
	err := s.pool.QueryRow(ctx, taskSelect+` WHERE id=$1 AND tenant_id=$2`, id, tenantID,
	).Scan(&t.ID, &t.TenantID, &t.Name, &t.Description, &t.ValueMinor, &t.Active, &t.IsBounty, &t.ClaimedBy, &t.ClaimedAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// ListTasks returns the tenant's tasks. When activeOnly is set (the holder
// view), retired tasks and already-claimed bounties are excluded.
func (s *Store) ListTasks(ctx context.Context, tenantID string, activeOnly bool) ([]domain.Task, error) {
	query := taskSelect + ` WHERE tenant_id=$1`
	if activeOnly {
		query += ` AND active = true AND (NOT is_bounty OR claimed_by IS NULL)`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Task
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Description, &t.ValueMinor, &t.Active, &t.IsBounty, &t.ClaimedBy, &t.ClaimedAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateTask updates the mutable fields (name, description, value, active).
func (s *Store) UpdateTask(ctx context.Context, t *domain.Task) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE tasks SET name=$3, description=$4, value_minor=$5, active=$6
		WHERE id=$1 AND tenant_id=$2`,
		t.ID, t.TenantID, t.Name, t.Description, t.ValueMinor, t.Active)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ClaimBounty atomically marks a bounty task as claimed by a customer, so two
// holders can't both win the same one-time bounty. Returns ErrConflict if it
// isn't an unclaimed bounty.
func (s *Store) ClaimBounty(ctx context.Context, tenantID, taskID, customerID string) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE tasks SET claimed_by=$3, claimed_at=now()
		WHERE id=$1 AND tenant_id=$2 AND is_bounty AND active AND claimed_by IS NULL`,
		taskID, tenantID, customerID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// ReleaseBountyClaim reopens a bounty after its claiming submission is voided,
// so another (or the same) holder can claim it again.
func (s *Store) ReleaseBountyClaim(ctx context.Context, tenantID, taskID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tasks SET claimed_by=NULL, claimed_at=NULL
		WHERE id=$1 AND tenant_id=$2 AND is_bounty`,
		taskID, tenantID)
	return err
}
