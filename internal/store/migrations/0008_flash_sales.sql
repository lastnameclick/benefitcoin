-- Flash sales: an operator-configured, time-boxed discount on redemptions —
-- either a percentage or a fixed amount off the standard 1-coin reward price.
-- Multiple sales can be scheduled per household, but their windows may not
-- overlap (enforced in the application layer) so the "currently active" sale
-- is always unambiguous.

CREATE TABLE flash_sales (
    id                   UUID PRIMARY KEY,
    tenant_id            UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    discount_type        TEXT NOT NULL CHECK (discount_type IN ('percent', 'fixed')),
    percent_off          INTEGER CHECK (percent_off BETWEEN 1 AND 99),
    amount_off_minor     BIGINT CHECK (amount_off_minor > 0),
    starts_at            TIMESTAMPTZ NOT NULL,
    ends_at              TIMESTAMPTZ NOT NULL CHECK (ends_at > starts_at),
    canceled_at          TIMESTAMPTZ,
    started_notified_at  TIMESTAMPTZ,
    created_by           UUID NOT NULL REFERENCES identities (id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (discount_type = 'percent' AND percent_off IS NOT NULL AND amount_off_minor IS NULL) OR
        (discount_type = 'fixed' AND amount_off_minor IS NOT NULL AND percent_off IS NULL)
    )
);

CREATE INDEX flash_sales_tenant_window_idx ON flash_sales (tenant_id, starts_at, ends_at) WHERE canceled_at IS NULL;
