-- In-app notification feed and Web Push subscriptions, keyed by identity (the
-- logged-in principal) rather than account, since operators receive
-- notifications (e.g. "redemption requested") without owning a ledger account.
-- Every notification is persisted here regardless of whether push delivery
-- succeeds, so the in-app bell always works with zero VAPID setup (same
-- philosophy as statements/mail).

CREATE TABLE notifications (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    identity_id UUID NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
    type        TEXT NOT NULL,
    title       TEXT NOT NULL,
    body        TEXT NOT NULL,
    data        JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at     TIMESTAMPTZ
);

CREATE INDEX notifications_identity_idx ON notifications (identity_id, read_at, created_at DESC);

CREATE TABLE push_subscriptions (
    id           UUID PRIMARY KEY,
    tenant_id    UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    identity_id  UUID NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
    endpoint     TEXT NOT NULL UNIQUE,
    p256dh       TEXT NOT NULL,
    auth         TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX push_subscriptions_identity_idx ON push_subscriptions (identity_id);

-- Tracks whether an unclaimed bounty has already had its "expiring soon" alert
-- sent, so the periodic sweep doesn't re-notify on every tick until it expires.
ALTER TABLE tasks ADD COLUMN expiring_notified_at TIMESTAMPTZ;
