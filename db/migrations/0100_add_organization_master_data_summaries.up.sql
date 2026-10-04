-- Enables the per-client financial summary panel on the redesigned Clients
-- screen. When off (0) the summary endpoints return a validation error.
-- Defaults to on (1) so every organization gets the feature immediately.
ALTER TABLE organizations ADD COLUMN masterDataSummaries INTEGER NOT NULL DEFAULT 1;