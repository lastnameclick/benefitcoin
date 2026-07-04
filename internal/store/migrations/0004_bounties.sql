-- One-time bounties: a task an operator posts that any holder can claim, but
-- only one holder at a time — first to submit it wins until it's decided.
-- Optionally timeboxed: once expires_at passes, it's hidden and unclaimable
-- even if nobody ever claimed it.

ALTER TABLE tasks
    ADD COLUMN is_bounty  BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN claimed_by UUID REFERENCES customers (id),
    ADD COLUMN claimed_at TIMESTAMPTZ,
    ADD COLUMN expires_at TIMESTAMPTZ;

CREATE INDEX tasks_bounty_idx ON tasks (tenant_id) WHERE is_bounty;
