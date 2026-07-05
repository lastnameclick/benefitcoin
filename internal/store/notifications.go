package store

import (
	"context"

	"cpal/internal/domain"
)

// InsertNotification persists one notification-feed entry.
func (s *Store) InsertNotification(ctx context.Context, n *domain.Notification) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO notifications (id, tenant_id, identity_id, type, title, body, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`,
		n.ID, n.TenantID, n.IdentityID, n.Type, n.Title, n.Body, n.Data,
	).Scan(&n.CreatedAt)
}

const notificationSelect = `
	SELECT id, tenant_id, identity_id, type, title, body, data, created_at, read_at
	FROM notifications`

// ListNotifications returns an identity's notifications, most recent first.
func (s *Store) ListNotifications(ctx context.Context, tenantID, identityID string, limit int) ([]domain.Notification, error) {
	rows, err := s.pool.Query(ctx, notificationSelect+`
		WHERE tenant_id=$1 AND identity_id=$2 ORDER BY created_at DESC LIMIT $3`,
		tenantID, identityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Notification
	for rows.Next() {
		var n domain.Notification
		if err := rows.Scan(&n.ID, &n.TenantID, &n.IdentityID, &n.Type, &n.Title, &n.Body, &n.Data, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CountUnreadNotifications returns how many of an identity's notifications are unread.
func (s *Store) CountUnreadNotifications(ctx context.Context, tenantID, identityID string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM notifications WHERE tenant_id=$1 AND identity_id=$2 AND read_at IS NULL`,
		tenantID, identityID).Scan(&n)
	return n, err
}

// MarkNotificationRead marks one notification read, scoped to its owner.
func (s *Store) MarkNotificationRead(ctx context.Context, tenantID, identityID, id string) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE notifications SET read_at=now()
		WHERE id=$1 AND tenant_id=$2 AND identity_id=$3 AND read_at IS NULL`,
		id, tenantID, identityID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkAllNotificationsRead marks every unread notification for an identity read.
func (s *Store) MarkAllNotificationsRead(ctx context.Context, tenantID, identityID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications SET read_at=now()
		WHERE tenant_id=$1 AND identity_id=$2 AND read_at IS NULL`,
		tenantID, identityID)
	return err
}

// UpsertPushSubscription stores (or refreshes) a Web Push subscription. The
// endpoint is globally unique, so re-subscribing the same browser just
// updates its keys and identity ownership.
func (s *Store) UpsertPushSubscription(ctx context.Context, sub *domain.PushSubscription) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO push_subscriptions (id, tenant_id, identity_id, endpoint, p256dh, auth)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (endpoint) DO UPDATE SET
			tenant_id=EXCLUDED.tenant_id, identity_id=EXCLUDED.identity_id,
			p256dh=EXCLUDED.p256dh, auth=EXCLUDED.auth
		RETURNING id, created_at`,
		sub.ID, sub.TenantID, sub.IdentityID, sub.Endpoint, sub.P256dh, sub.Auth,
	).Scan(&sub.ID, &sub.CreatedAt)
}

// DeletePushSubscription removes a subscription by endpoint, scoped to its owner.
func (s *Store) DeletePushSubscription(ctx context.Context, tenantID, identityID, endpoint string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM push_subscriptions WHERE endpoint=$1 AND tenant_id=$2 AND identity_id=$3`,
		endpoint, tenantID, identityID)
	return err
}

// DeletePushSubscriptionByEndpoint removes a subscription regardless of owner
// — used when a push provider reports the endpoint as gone (410/404).
func (s *Store) DeletePushSubscriptionByEndpoint(ctx context.Context, endpoint string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE endpoint=$1`, endpoint)
	return err
}

// ListPushSubscriptions returns every push subscription registered for an identity.
func (s *Store) ListPushSubscriptions(ctx context.Context, tenantID, identityID string) ([]domain.PushSubscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, identity_id, endpoint, p256dh, auth, created_at, last_used_at
		FROM push_subscriptions WHERE tenant_id=$1 AND identity_id=$2`,
		tenantID, identityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PushSubscription
	for rows.Next() {
		var p domain.PushSubscription
		if err := rows.Scan(&p.ID, &p.TenantID, &p.IdentityID, &p.Endpoint, &p.P256dh, &p.Auth, &p.CreatedAt, &p.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
