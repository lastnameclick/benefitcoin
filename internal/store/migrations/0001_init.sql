-- Core-banking metadata schema. TigerBeetle holds the authoritative ledger
-- (balances + postings); these tables hold everything human-readable and the
-- transaction envelopes that reference TigerBeetle transfer ids.

CREATE TABLE customers (
    id           UUID PRIMARY KEY,
    type         TEXT NOT NULL CHECK (type IN ('operator', 'holder')),
    display_name TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'active',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE identities (
    id            UUID PRIMARY KEY,
    customer_id   UUID NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('operator', 'holder')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE accounts (
    id            UUID PRIMARY KEY,
    customer_id   UUID REFERENCES customers (id) ON DELETE CASCADE, -- NULL for internal GL
    kind          TEXT NOT NULL CHECK (kind IN ('customer', 'internal')),
    tb_account_id TEXT NOT NULL UNIQUE,
    currency      TEXT NOT NULL DEFAULT 'BNC',
    product       TEXT NOT NULL DEFAULT 'benefit-coin',
    name          TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'open',
    opened_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX accounts_customer_idx ON accounts (customer_id);

CREATE TABLE tasks (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    value_minor BIGINT NOT NULL CHECK (value_minor > 0),
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE transactions (
    id                     UUID PRIMARY KEY,
    type                   TEXT NOT NULL CHECK (type IN ('earn', 'redeem')),
    status                 TEXT NOT NULL CHECK (status IN ('pending', 'settled', 'voided')),
    account_id             UUID NOT NULL REFERENCES accounts (id),
    gl_account_id          UUID NOT NULL REFERENCES accounts (id),
    amount_minor           BIGINT NOT NULL CHECK (amount_minor > 0),
    task_id                UUID REFERENCES tasks (id),
    memo                   TEXT NOT NULL DEFAULT '',
    tb_pending_transfer_id TEXT NOT NULL,
    tb_post_transfer_id    TEXT,
    created_by             UUID NOT NULL REFERENCES identities (id),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_by             UUID REFERENCES identities (id),
    decided_at             TIMESTAMPTZ
);
CREATE INDEX transactions_status_idx ON transactions (status);
CREATE INDEX transactions_account_idx ON transactions (account_id, created_at DESC);

-- Idempotency keys make every write safe to retry. Scoped per identity so two
-- users can't collide on the same client-chosen key.
CREATE TABLE idempotency_keys (
    identity_id   UUID NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
    key           TEXT NOT NULL,
    method        TEXT NOT NULL,
    path          TEXT NOT NULL,
    request_hash  TEXT NOT NULL,
    status_code   INT NOT NULL,
    response_body JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (identity_id, key)
);

CREATE TABLE refresh_tokens (
    id          UUID PRIMARY KEY,
    identity_id UUID NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Append-only audit trail of every state-changing action.
CREATE TABLE audit_events (
    id                UUID PRIMARY KEY,
    actor_identity_id UUID REFERENCES identities (id),
    action            TEXT NOT NULL,
    entity_type       TEXT NOT NULL,
    entity_id         TEXT NOT NULL,
    metadata          JSONB NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_created_idx ON audit_events (created_at DESC);
