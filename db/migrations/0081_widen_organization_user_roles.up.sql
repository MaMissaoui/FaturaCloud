-- Widens organization_users.role from a binary admin/user flag to six real
-- per-org roles: admin (unchanged), general (a rename of the old "user" —
-- keeps identical access, nobody is tightened by this migration), and four
-- narrow domain roles (sales, purchasing, accounting, cashbook) each scoped
-- to write access within their own area only (enforced in application code,
-- api/router.go) while still reading everything, same as every member
-- already could. SQLite can't ALTER a CHECK constraint in place, so this
-- rebuilds the table the same way any CHECK-constraint change on SQLite
-- requires: new table, copy, drop, rename.
CREATE TABLE organization_users_new (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    userId TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'general'
        CHECK (role IN ('admin', 'general', 'sales', 'purchasing', 'accounting', 'cashbook')),
    createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now'))
);

INSERT INTO organization_users_new (id, organizationId, userId, role, createdAt)
SELECT id, organizationId, userId,
       CASE WHEN role = 'user' THEN 'general' ELSE role END,
       createdAt
FROM organization_users;

DROP TABLE organization_users;
ALTER TABLE organization_users_new RENAME TO organization_users;

CREATE UNIQUE INDEX IF NOT EXISTS organization_users_org_user
    ON organization_users(organizationId, userId);
CREATE INDEX IF NOT EXISTS organization_users_user ON organization_users(userId);
