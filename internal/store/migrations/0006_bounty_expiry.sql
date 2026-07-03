-- expires_at was folded into 0004_bounties.sql after some databases had
-- already run it without that column. Add it defensively so this applies
-- cleanly whether or not 0004 already created it.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
