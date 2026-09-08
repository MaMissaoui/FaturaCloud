-- Per-organization membership: today any authenticated user can read/write
-- every organization's data, and users.role is a single global admin/user
-- flag. This table makes organization access an explicit grant with its own
-- role, so a user only sees/acts on organizations they're a member of.
CREATE TABLE IF NOT EXISTS organization_users (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    userId TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS organization_users_org_user
    ON organization_users(organizationId, userId);
CREATE INDEX IF NOT EXISTS organization_users_user ON organization_users(userId);

-- isPlatformAdmin is orthogonal to any organization membership: it gates the
-- handful of genuinely global actions (user account management, backups, DB
-- restore, countries) that have no natural per-org owner. users.role is left
-- in place for now (still read by provisionOrSyncUser's OIDC sync and
-- countActiveAdmins) rather than dropped in the same migration that
-- introduces its replacement.
ALTER TABLE users ADD COLUMN isPlatformAdmin INTEGER NOT NULL DEFAULT 0
    CHECK (isPlatformAdmin IN (0, 1));

-- Backfill: preserve exactly today's access so nobody is locked out on
-- deploy — every existing user gets a membership row (at their current
-- global role) for every existing organization, and today's admins become
-- platform admins too. Only new orgs/users going forward need explicit
-- grants.
UPDATE users SET isPlatformAdmin = 1 WHERE role = 'admin';

INSERT INTO organization_users (id, organizationId, userId, role, createdAt)
SELECT lower(hex(randomblob(16))), o.id, u.id, u.role, strftime('%Y-%m-%d %H:%M:%S', 'now')
FROM organizations o
CROSS JOIN users u;
