-- A maintained per-organization list of payment terms (e.g. "Net 30", "Due
-- on receipt"), so invoices.paymentTerms is picked from a Select instead of
-- free-typed on every invoice. invoices.paymentTerms itself stays a plain
-- TEXT column storing whatever name was selected (or was already stored
-- before this table existed) — there's no FK, so deleting a payment term
-- later never orphans an existing invoice's stored value.
CREATE TABLE IF NOT EXISTS payment_terms (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    isDefault INTEGER NOT NULL DEFAULT 0 CHECK (isDefault IN (0, 1)),
    createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS payment_terms_org_name
    ON payment_terms(organizationId, name);
