-- invoiceLayout (0062) was an invoice-only selector for the client-side
-- React-PDF templates and went inert when every PDF moved to the server-side
-- Excel template path. It's revived here as documentLayout: the per-org
-- choice of which embedded template set (db/templates_embed.go) every
-- document type's export uses when the org hasn't uploaded an override —
-- 'tunisia' selects the Tunisian "Facture" layout; NULL, '', 'default' or
-- anything else selects the generic default layout.
ALTER TABLE organizations RENAME COLUMN invoiceLayout TO documentLayout;

-- The old selector's 'custom' option never did anything real (an uploaded
-- document_templates override is detected on its own), so it reads as the
-- default layout from here on.
UPDATE organizations SET documentLayout = NULL WHERE documentLayout = 'custom';
