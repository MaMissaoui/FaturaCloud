-- The activity history (api/audit.go): one row per successful change made
-- through the API — who, when, which route, which document — kept for two
-- years (PruneAuditEvents). organizationId is NULL for a platform action
-- (users, backups, restore) and deliberately has no foreign key: the
-- history of a deleted organization stays until it ages out, and the row
-- recording the deletion itself can still be written. The user is a
-- SET NULL link plus a copy of their email, so deleting a user keeps their
-- history readable.
CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    createdAt INTEGER NOT NULL,
    organizationId TEXT,
    userId TEXT REFERENCES users(id) ON DELETE SET NULL,
    userEmail TEXT NOT NULL DEFAULT '',
    method TEXT NOT NULL,
    route TEXT NOT NULL,
    resource TEXT NOT NULL DEFAULT '',
    entityId TEXT NOT NULL DEFAULT '',
    entityLabel TEXT NOT NULL DEFAULT '',
    fromState TEXT NOT NULL DEFAULT '',
    toState TEXT NOT NULL DEFAULT '',
    requestId TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_audit_events_org_created ON audit_events (organizationId, createdAt DESC);
CREATE INDEX idx_audit_events_entity ON audit_events (entityId);
CREATE INDEX idx_audit_events_created ON audit_events (createdAt);
