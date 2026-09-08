-- Issue #115: per-organization, per-document-type Excel template overrides.
-- One row per (organizationId, documentType); absence of a row means "use the
-- embedded default template" (see db.resolveTemplateBytes). ON DELETE CASCADE
-- (unlike vendorId/importId's deliberate no-cascade precedent elsewhere in
-- this schema) because a template override has no independent lifecycle
-- worth guarding — deleting the organization should take it with it.
CREATE TABLE IF NOT EXISTS document_templates (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    documentType TEXT NOT NULL,
    filename TEXT NOT NULL,
    content BLOB NOT NULL,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
    updatedAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);

CREATE UNIQUE INDEX IF NOT EXISTS document_templates_org_doctype
    ON document_templates(organizationId, documentType);
