-- Per-organization, per-document-type page orientation preference for the
-- Excel/PDF export engine (db/xlsx_export.go). Absence of a row means "no
-- override" — the fill engine leaves the template's own authored page setup
-- untouched, so every existing organization keeps byte-identical output
-- until it explicitly opts in via this table (see db.fillTemplate). This is
-- a separate table from document_templates (issue #115) rather than a new
-- nullable column there: document_templates.content is NOT NULL and a row
-- there only ever exists when an org has uploaded an override file — making
-- content nullable would break resolveTemplateBytes' "no row means use the
-- embedded default" contract that several tests and CLAUDE.md encode.
CREATE TABLE IF NOT EXISTS document_template_settings (
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    documentType TEXT NOT NULL,
    orientation TEXT NOT NULL CHECK (orientation IN ('portrait', 'landscape')),
    updatedAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
    PRIMARY KEY (organizationId, documentType)
);
