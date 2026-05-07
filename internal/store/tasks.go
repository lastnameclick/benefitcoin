package store

import (
	"context"
	"errors"

	"cpal/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateTask(ctx context.Context, t *domain.Task) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO tasks (id, tenant_id, name, description, value_minor, active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`,
		t.ID, t.TenantID, t.Name, t.Description, t.ValueMinor, t.Active,
	).Scan(&t.CreatedAt)
}

func (s *Store) GetTask(ctx context.Context, tenantID, id string) (domain.Task, error) {
	var t domain.Task
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, description, value_minor, active, created_at
		FROM tasks WHERE id=$1 AND tenant_id=$2`, id, tenantID,
	).Scan(&t.ID, &t.TenantID, &t.Name, &t.Description, &t.ValueMinor, &t.Active, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

func (s *Store) ListTasks(ctx context.Context, tenantID string, activeOnly bool) ([]domain.Task, error) {
	query := `SELECT id, tenant_id, name, description, value_minor, active, created_at
		FROM tasks WHERE tenant_id=$1`
	if activeOnly {
		query += ` AND active = true`
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
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Description, &t.ValueMinor, &t.Active, &t.CreatedAt); err != nil {
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
