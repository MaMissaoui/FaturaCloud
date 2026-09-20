-- Performance indexes for hot read paths surfaced by the 2026-09-20 DB audit.
-- Index-only migration, no schema change (same shape as 0060/0084).

-- 1. recomputeAverageCostTx (db/product_cost.go) replays a product's entire
--    movement history ORDER BY createdAt ASC, rowid ASC on every stock write
--    (receipt, shipment, adjustment, movement delete). The existing
--    idx_stockMovements_productId alone forced a temp B-tree sort of that
--    history; this composite serves the ordering directly (and also removes
--    the sort from GetProductStockMovements, db/stock.go).
CREATE INDEX IF NOT EXISTS idx_stockMovements_productId_createdAt
    ON stockMovements(productId, createdAt);

-- 2. Name-ordered list pages (products/clients/vendors/taxRates) filter by
--    organizationId and then ORDER BY name. Each had only a single-column
--    organizationId index, so the ORDER BY fell to a temp B-tree sort on
--    every list load. The composite's leading organizationId still serves
--    organization-only filters, so the old index stays valuable but these
--    add the ordering for free.
CREATE INDEX IF NOT EXISTS idx_products_organizationId_name
    ON products(organizationId, name);
CREATE INDEX IF NOT EXISTS idx_clients_organizationId_name
    ON clients(organizationId, name);
CREATE INDEX IF NOT EXISTS idx_vendors_organizationId_name
    ON vendors(organizationId, name);
CREATE INDEX IF NOT EXISTS idx_taxRates_organizationId_name
    ON taxRates(organizationId, name);

-- 3. GetIncomingInvoiceMatchSummaries (db/incoming_invoice_match.go) drives a
--    query from inbound_deliveries filtered by organizationId AND
--    status = 'received', which previously scanned the table.
CREATE INDEX IF NOT EXISTS inbound_deliveries_organizationId_status
    ON inbound_deliveries(organizationId, status);

-- 4. Usage-count delete guards. db/account.go's GetAccountUsageCount and
--    db/tax_rate.go's GetTaxRateUsageCount each SUM/COUNT over an FK column
--    that had no index, so an admin confirming a delete ran a full scan of a
--    potentially large table. These are rare operations, but the indexes are
--    small and also help the FK lookup on delete.
CREATE INDEX IF NOT EXISTS payments_bankAccountId
    ON payments(bankAccountId);
CREATE INDEX IF NOT EXISTS products_revenueAccountId
    ON products(revenueAccountId);
CREATE INDEX IF NOT EXISTS products_expenseAccountId
    ON products(expenseAccountId);
CREATE INDEX IF NOT EXISTS products_taxRateId
    ON products(taxRateId);
CREATE INDEX IF NOT EXISTS purchase_order_line_items_taxRate
    ON purchase_order_line_items(taxRate);
CREATE INDEX IF NOT EXISTS incoming_invoice_line_items_taxRate
    ON incoming_invoice_line_items(taxRate);
CREATE INDEX IF NOT EXISTS journal_lines_taxRateId
    ON journal_lines(taxRateId);
