-- Nullable: an org can use the chart of accounts + manual entries before
-- auto-posting is wired up; auto-posting is refused with a 409 (not a 500)
-- when a default it needs is unset. Seeded by seedDefaultChartOfAccounts
-- (db/account.go) for every organization once accounts exist for it.
ALTER TABLE organizations ADD COLUMN defaultArAccountId TEXT REFERENCES accounts(id);
ALTER TABLE organizations ADD COLUMN defaultApAccountId TEXT REFERENCES accounts(id);
ALTER TABLE organizations ADD COLUMN defaultRevenueAccountId TEXT REFERENCES accounts(id);
ALTER TABLE organizations ADD COLUMN defaultExpenseAccountId TEXT REFERENCES accounts(id);
ALTER TABLE organizations ADD COLUMN defaultCashAccountId TEXT REFERENCES accounts(id);
ALTER TABLE organizations ADD COLUMN fxGainAccountId TEXT REFERENCES accounts(id);
ALTER TABLE organizations ADD COLUMN fxLossAccountId TEXT REFERENCES accounts(id);
ALTER TABLE organizations ADD COLUMN retainedEarningsAccountId TEXT REFERENCES accounts(id);
-- DATEV multi-line Gegenkonto fallback for manual entries with more than one
-- line per side (see the design decision in the accounting implementation plan).
ALTER TABLE organizations ADD COLUMN datevClearingAccountId TEXT REFERENCES accounts(id);
ALTER TABLE organizations ADD COLUMN datev_consultant_number TEXT;
ALTER TABLE organizations ADD COLUMN datev_client_number TEXT;
-- FEC's SIREN is NOT duplicated here — reused from organizations.registration_number.
