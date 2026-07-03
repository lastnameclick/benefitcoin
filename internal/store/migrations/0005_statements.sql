-- Generated PDF statements. Every statement is persisted here regardless of
-- whether it's ever emailed, so the in-app Inbox works with zero SMTP setup.

CREATE TABLE statements (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    account_id  UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    period      DATE NOT NULL, -- first day of the statement month
    pdf         BYTEA NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    emailed_at  TIMESTAMPTZ,
    viewed_at   TIMESTAMPTZ
);

CREATE INDEX statements_account_idx ON statements (tenant_id, account_id, period DESC);
