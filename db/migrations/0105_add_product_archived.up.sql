-- Retires a product without deleting it (audit F167). A product with any
-- history — stock movements, document lines, recipes — can no longer be
-- deleted, since that erased its stock history while the GL kept it; it is
-- archived instead: hidden from the product pickers and, by default, from
-- the Products list, but kept on its documents and in every stock report.
-- Every existing product stays active (0).
ALTER TABLE products ADD COLUMN isArchived INTEGER NOT NULL DEFAULT 0;
