-- Org-level default (organizations.defaultRevenueAccountId/defaultExpenseAccountId)
-- plus a per-product override — identical shape to products.taxRateId
-- overriding taxRates.isDefault, no new pattern invented.
ALTER TABLE products ADD COLUMN revenueAccountId TEXT REFERENCES accounts(id);
ALTER TABLE products ADD COLUMN expenseAccountId TEXT REFERENCES accounts(id);
