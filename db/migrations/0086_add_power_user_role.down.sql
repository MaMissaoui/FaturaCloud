-- Lossy by necessity: "power_user" was introduced as a copy of "general"
-- (the full non-admin role), so it collapses back to "general" on the way
-- down — the closest equivalent the pre-0086 role set has.
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
       CASE WHEN role = 'power_user' THEN 'general' ELSE role END,
       createdAt
FROM organization_users;

DROP TABLE organization_users;
ALTER TABLE organization_users_new RENAME TO organization_users;

CREATE UNIQUE INDEX IF NOT EXISTS organization_users_org_user
    ON organization_users(organizationId, userId);
CREATE INDEX IF NOT EXISTS organization_users_user ON organization_users(userId);
