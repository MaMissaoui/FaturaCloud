-- Adds the "power_user" role — a copy of the current "general" role's full
-- non-admin access. This pairs with the change that narrows "general" (it
-- loses Accounting, Imports and Bill of Materials), so an organization can
-- still grant a non-admin member the full access "general" used to carry.
--
-- SQLite can't ALTER a CHECK constraint in place, so this rebuilds the table
-- the same way migration 0081 did: new table, copy, drop, rename.
CREATE TABLE organization_users_new (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    userId TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'general'
        CHECK (role IN ('admin', 'general', 'power_user', 'sales', 'purchasing', 'accounting', 'cashbook')),
    createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now'))
);

INSERT INTO organization_users_new (id, organizationId, userId, role, createdAt)
SELECT id, organizationId, userId, role, createdAt
FROM organization_users;

DROP TABLE organization_users;
ALTER TABLE organization_users_new RENAME TO organization_users;

CREATE UNIQUE INDEX IF NOT EXISTS organization_users_org_user
    ON organization_users(organizationId, userId);
CREATE INDEX IF NOT EXISTS organization_users_user ON organization_users(userId);
