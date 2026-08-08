-- Phase 7 (inventory/COGS GL integration). Four new org-level GL defaults,
-- nullable — same convention as every default*AccountId column added in
-- 0054: an organization can keep tracking stock without inventory GL wired
-- up until ready, and auto-posting refuses with a 409 (not a 500) until
-- then. Seeded for new organizations by seedDefaultChartOfAccounts and
-- backfilled for existing ones by SeedInventoryAccountingDefaultsForAllOrganizations
-- (db/account.go) — the account-count-gated backfill that already runs at
-- startup can't reach these, since every organization that ran Phases 1-6
-- already has a non-empty chart.
--
-- No per-product override (unlike defaultRevenueAccountId/defaultExpenseAccountId
-- in 0055) — one Inventory/GRNI/COGS/Adjustment account per organization.
ALTER TABLE organizations ADD COLUMN defaultInventoryAccountId TEXT REFERENCES accounts(id);
ALTER TABLE organizations ADD COLUMN defaultGRNIAccountId TEXT REFERENCES accounts(id);
ALTER TABLE organizations ADD COLUMN defaultCOGSAccountId TEXT REFERENCES accounts(id);
ALTER TABLE organizations ADD COLUMN defaultInventoryAdjustmentAccountId TEXT REFERENCES accounts(id);
