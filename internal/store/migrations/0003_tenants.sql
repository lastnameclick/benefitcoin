-- Multi-tenancy: a tenant is a household, the isolation boundary. Every
-- tenant-scoped table gains a tenant_id, and each tenant keeps its own
-- Issuance/Redemption GL accounts so its books balance independently.

CREATE TABLE tenants (
    id                    UUID PRIMARY KEY,
    name                  TEXT NOT NULL,
    status                TEXT NOT NULL DEFAULT 'active',
    issuance_account_id   UUID,        -- Postgres GL account row (set at signup)
    redemption_account_id UUID,        -- Postgres GL account row (set at signup)
    issuance_tb_id        TEXT NOT NULL,
    redemption_tb_id      TEXT NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A default household adopts any pre-existing single-tenant data. It reuses the
-- original global GL TigerBeetle accounts (ids 1 and 2).
INSERT INTO tenants (id, name, issuance_tb_id, redemption_tb_id)
VALUES ('00000000-0000-0000-0000-0000000000aa', 'Default Household', '1', '2');

ALTER TABLE customers    ADD COLUMN tenant_id UUID;
ALTER TABLE identities   ADD COLUMN tenant_id UUID;
ALTER TABLE accounts     ADD COLUMN tenant_id UUID;
ALTER TABLE tasks        ADD COLUMN tenant_id UUID;
ALTER TABLE transactions ADD COLUMN tenant_id UUID;
ALTER TABLE audit_events ADD COLUMN tenant_id UUID;

-- Backfill existing rows into the default household.
UPDATE customers    SET tenant_id = '00000000-0000-0000-0000-0000000000aa' WHERE tenant_id IS NULL;
UPDATE identities   SET tenant_id = '00000000-0000-0000-0000-0000000000aa' WHERE tenant_id IS NULL;
UPDATE accounts     SET tenant_id = '00000000-0000-0000-0000-0000000000aa' WHERE tenant_id IS NULL;
UPDATE tasks        SET tenant_id = '00000000-0000-0000-0000-0000000000aa' WHERE tenant_id IS NULL;
UPDATE transactions SET tenant_id = '00000000-0000-0000-0000-0000000000aa' WHERE tenant_id IS NULL;
UPDATE audit_events SET tenant_id = '00000000-0000-0000-0000-0000000000aa' WHERE tenant_id IS NULL;

-- Wire the default household to its GL account rows, if they were bootstrapped.
UPDATE tenants SET
    issuance_account_id   = (SELECT id FROM accounts WHERE tb_account_id = '1' LIMIT 1),
    redemption_account_id = (SELECT id FROM accounts WHERE tb_account_id = '2' LIMIT 1)
WHERE id = '00000000-0000-0000-0000-0000000000aa';

ALTER TABLE customers    ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE identities   ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE accounts     ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE tasks        ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE transactions ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE audit_events ALTER COLUMN tenant_id SET NOT NULL;

ALTER TABLE customers    ADD CONSTRAINT customers_tenant_fk    FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE;
ALTER TABLE identities   ADD CONSTRAINT identities_tenant_fk   FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE;
ALTER TABLE accounts     ADD CONSTRAINT accounts_tenant_fk     FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE;
ALTER TABLE tasks        ADD CONSTRAINT tasks_tenant_fk        FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE;
ALTER TABLE transactions ADD CONSTRAINT transactions_tenant_fk FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE;
ALTER TABLE audit_events ADD CONSTRAINT audit_tenant_fk        FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE;

CREATE INDEX customers_tenant_idx    ON customers (tenant_id);
CREATE INDEX identities_tenant_idx   ON identities (tenant_id);
CREATE INDEX accounts_tenant_idx     ON accounts (tenant_id);
CREATE INDEX tasks_tenant_idx        ON tasks (tenant_id);
CREATE INDEX transactions_tenant_idx ON transactions (tenant_id, created_at DESC);
CREATE INDEX audit_events_tenant_idx ON audit_events (tenant_id, created_at DESC);
