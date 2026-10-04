-- Marks an organization as a test/demo organization (seeded or sandbox data,
-- not a real business). Purely a visible marker: the header and the
-- Organizations list show a "Test" tag so nobody mistakes it for a real
-- organization. No behaviour depends on it. Every existing organization
-- stays a real one (0).
ALTER TABLE organizations ADD COLUMN isTest INTEGER NOT NULL DEFAULT 0;
