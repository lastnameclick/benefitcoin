package store

import (
	"context"

	"cpal/internal/domain"
)

func (s *Store) InsertAudit(ctx context.Context, e *domain.AuditEvent) error {
	meta := e.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	return s.pool.QueryRow(ctx, `
		INSERT INTO audit_events (id, tenant_id, actor_identity_id, action, entity_type, entity_id, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING created_at`,
		e.ID, e.TenantID, e.ActorIdentityID, e.Action, e.EntityType, e.EntityID, meta,
	).Scan(&e.CreatedAt)
}

func (s *Store) ListAudit(ctx context.Context, tenantID string, limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, actor_identity_id, action, entity_type, entity_id, metadata, created_at
		FROM audit_events WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AuditEvent
	for rows.Next() {
		var e domain.AuditEvent
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ActorIdentityID, &e.Action, &e.EntityType,
			&e.EntityID, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
