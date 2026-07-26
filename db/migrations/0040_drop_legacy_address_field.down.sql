-- Data loss on rollback is accepted here: the backfill direction (structured
-- -> free text) can't be reconstructed losslessly, and this app is not yet
-- in production.
ALTER TABLE clients ADD COLUMN address TEXT;
ALTER TABLE organizations ADD COLUMN address TEXT;
ALTER TABLE vendors ADD COLUMN address TEXT;
