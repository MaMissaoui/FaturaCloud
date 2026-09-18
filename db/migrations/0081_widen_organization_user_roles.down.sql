-- Lossy by necessity: a domain role (sales/purchasing/accounting/cashbook)
-- has no equivalent in the old binary admin/user scheme, so it collapses to
-- "user" — the same broad access every non-admin member had before this
-- migration existed, just no longer distinguishing which domain they were
-- scoped to.
CREATE TABLE organization_users_new (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    userId TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now'))
);

INSERT INTO organization_users_new (id, organizationId, userId, role, createdAt)
SELECT id, organizationId, userId,
       CASE WHEN role = 'admin' THEN 'admin' ELSE 'user' END,
       createdAt
FROM organization_users;

DROP TABLE organization_users;
ALTER TABLE organization_users_new RENAME TO organization_users;

CREATE UNIQUE INDEX IF NOT EXISTS organization_users_org_user
    ON organization_users(organizationId, userId);
CREATE INDEX IF NOT EXISTS organization_users_user ON organization_users(userId);
