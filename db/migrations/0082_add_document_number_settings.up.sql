-- Per-organization, per-document-type numbering configuration (format +
-- persistent counter), the same shape document_template_settings already
-- established for a different per-(org, documentType) setting. Before this,
-- only invoices had a configurable format (organizations.invoice_number_format/
-- invoice_number_counter) — orders, purchase orders, outbound/inbound
-- deliveries, and production orders all had a hardcoded prefix with no org
-- setting at all, and their "next number" was a MAX(...)+1 scan rather than
-- a persistent counter. Invoice numbering is deliberately left where it is
-- (organizations columns, unchanged) rather than migrated here — one more
-- source of truth for the other five types is a smaller, safer change than
-- rebuilding the organizations table to drop two columns.
CREATE TABLE IF NOT EXISTS document_number_settings (
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    documentType TEXT NOT NULL CHECK (
        documentType IN ('order', 'purchase_order', 'delivery', 'inbound_delivery', 'production_order')
    ),
    format TEXT NOT NULL,
    counter INTEGER NOT NULL DEFAULT 0,
    updatedAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
    PRIMARY KEY (organizationId, documentType)
);
