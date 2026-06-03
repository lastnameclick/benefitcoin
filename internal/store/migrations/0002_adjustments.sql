-- Operator-initiated balance adjustments (manual journal entries), plus
-- descriptive metadata that can be attached to any transaction.

ALTER TABLE transactions
    ADD COLUMN effective_at TIMESTAMPTZ,                 -- when it actually occurred
    ADD COLUMN details      JSONB NOT NULL DEFAULT '{}'; -- free-form detail (note, category, ...)

-- Allow the two new adjustment types alongside earn/redeem.
ALTER TABLE transactions DROP CONSTRAINT transactions_type_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_type_check
    CHECK (type IN ('earn', 'redeem', 'adjust_credit', 'adjust_debit'));
