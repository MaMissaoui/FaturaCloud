-- Theme support, Phase 1: a per-organization accent color, selected from a
-- small curated swatch set (brandColorPalette in db/organization.go is the
-- server-side source of truth — the API rejects anything else with a 409).
-- Nullable, same convention as every other optional organization field:
-- unset means "use antd's default blue". Empty string is a distinct,
-- deliberate "explicitly reset to default" value (mirrors the `code` column
-- convention) rather than reusing NULL, since UpdateOrganization's COALESCE
-- semantics mean a NULL in the request always means "leave unchanged", not
-- "clear this field".
ALTER TABLE organizations ADD COLUMN brandColor TEXT;
