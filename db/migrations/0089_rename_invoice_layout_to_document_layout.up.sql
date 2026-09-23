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

-- One-time continuity backfill: from v3.52.0 the Tunisian "Facture" layout
-- was every organization's only embedded default, so an unset layout now
-- means the generic one. Tunisian organizations keep the layout they were
-- already getting. This runs once at upgrade and is not runtime inference:
-- afterwards the layout is purely the explicit setting, as 0062 intended.
-- country_code is the ISO code (set from the Organizations drawer or
-- seed-demo); country is the free-text name, the only country field org
-- creation collects, so both are checked.
UPDATE organizations SET documentLayout = 'tunisia'
 WHERE (documentLayout IS NULL OR documentLayout = '')
   AND (UPPER(TRIM(country_code)) = 'TN'
        OR LOWER(TRIM(country)) IN ('tunisia', 'tunisie'));
