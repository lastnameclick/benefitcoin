package store

import (
	"context"
	"errors"
	"time"

	"cpal/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateTask(ctx context.Context, t *domain.Task) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO tasks (id, tenant_id, name, description, value_minor, active, is_bounty, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`,
		t.ID, t.TenantID, t.Name, t.Description, t.ValueMinor, t.Active, t.IsBounty, t.ExpiresAt,
	).Scan(&t.CreatedAt)
}

const taskSelect = `SELECT id, tenant_id, name, description, value_minor, active, is_bounty, claimed_by, claimed_at, expires_at, created_at FROM tasks`

func (s *Store) GetTask(ctx context.Context, tenantID, id string) (domain.Task, error) {
	var t domain.Task
	err := s.pool.QueryRow(ctx, taskSelect+` WHERE id=$1 AND tenant_id=$2`, id, tenantID,
	).Scan(&t.ID, &t.TenantID, &t.Name, &t.Description, &t.ValueMinor, &t.Active, &t.IsBounty, &t.ClaimedBy, &t.ClaimedAt, &t.ExpiresAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// ListTasks returns the tenant's tasks. When activeOnly is set (the holder
// view), retired tasks, already-claimed bounties, and expired bounties are
// excluded.
func (s *Store) ListTasks(ctx context.Context, tenantID string, activeOnly bool) ([]domain.Task, error) {
	query := taskSelect + ` WHERE tenant_id=$1`
	if activeOnly {
		query += ` AND active = true
			AND (NOT is_bounty OR (claimed_by IS NULL AND (expires_at IS NULL OR expires_at > now())))`
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
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Description, &t.ValueMinor, &t.Active, &t.IsBounty, &t.ClaimedBy, &t.ClaimedAt, &t.ExpiresAt, &t.CreatedAt); err != nil {
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
// holders can't both win the same one-time bounty. Returns ErrExpired if its
// deadline has passed, or ErrConflict if it's otherwise not an unclaimed bounty.
func (s *Store) ClaimBounty(ctx context.Context, tenantID, taskID, customerID string) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE tasks SET claimed_by=$3, claimed_at=now()
		WHERE id=$1 AND tenant_id=$2 AND is_bounty AND active AND claimed_by IS NULL
		  AND (expires_at IS NULL OR expires_at > now())`,
		taskID, tenantID, customerID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		task, gerr := s.GetTask(ctx, tenantID, taskID)
		if gerr == nil && task.ExpiresAt != nil && !task.ExpiresAt.After(time.Now()) {
			return ErrExpired
		}
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

// RetireExpiredBounties deactivates bounties whose deadline has passed but
// were never claimed, so the catalog doesn't need a manual "Retire" click for
// something that's already unclaimable. This only flips active — it never
// deletes the row, preserving history like a manual retire does. Returns the
// ids it retired, for auditing.
func (s *Store) RetireExpiredBounties(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE tasks SET active = false
		WHERE tenant_id=$1 AND is_bounty AND active
		  AND expires_at IS NOT NULL AND expires_at <= now()
		RETURNING id`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// RetireExpiredBountiesDetailed is RetireExpiredBounties, but returns the full
// task rows (not just ids) — used by the notification sweep, which needs
// claimed_by to decide who (if anyone) to alert about the expiry.
func (s *Store) RetireExpiredBountiesDetailed(ctx context.Context, tenantID string) ([]domain.Task, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE tasks SET active = false
		WHERE tenant_id=$1 AND is_bounty AND active
		  AND expires_at IS NOT NULL AND expires_at <= now()
		RETURNING id, tenant_id, name, description, value_minor, active, is_bounty, claimed_by, claimed_at, expires_at, created_at`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Task
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Description, &t.ValueMinor, &t.Active, &t.IsBounty,
			&t.ClaimedBy, &t.ClaimedAt, &t.ExpiresAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListBountiesExpiringSoon returns unclaimed, active bounties whose deadline
// falls within the given window and that haven't already been flagged, so a
// periodic sweep can send a one-time "expiring soon" alert.
func (s *Store) ListBountiesExpiringSoon(ctx context.Context, tenantID string, within time.Duration) ([]domain.Task, error) {
	rows, err := s.pool.Query(ctx, taskSelect+`
		WHERE tenant_id=$1 AND is_bounty AND active AND claimed_by IS NULL
		  AND expiring_notified_at IS NULL
		  AND expires_at IS NOT NULL AND expires_at > now() AND expires_at <= now() + $2::interval`,
		tenantID, within)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Task
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Description, &t.ValueMinor, &t.Active, &t.IsBounty,
			&t.ClaimedBy, &t.ClaimedAt, &t.ExpiresAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarkBountyExpiringNotified flags a bounty as having already sent its
// "expiring soon" alert, so the sweep doesn't repeat it on every tick.
func (s *Store) MarkBountyExpiringNotified(ctx context.Context, tenantID, taskID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tasks SET expiring_notified_at = now() WHERE id=$1 AND tenant_id=$2`,
		taskID, tenantID)
	return err
}
