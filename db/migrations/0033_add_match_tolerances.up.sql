-- Organization-level tolerance policy for 3-way matching. Defaults are zero:
-- any variance is flagged until the organization decides otherwise.
ALTER TABLE organizations ADD COLUMN match_price_tolerance_percent REAL NOT NULL DEFAULT 0;
ALTER TABLE organizations ADD COLUMN match_quantity_tolerance_percent REAL NOT NULL DEFAULT 0;
