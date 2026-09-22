-- Remise (discount) on sales invoices: a flat pre-tax amount in cents,
-- subtracted from the line-item subtotal before VAT is computed — the
-- Tunisian invoice convention the printed layout follows ("Total Brut HTVA
-- − Remise = Total Net HTVA", VAT charged on the net). NOT NULL DEFAULT 0,
-- the same convention as fiscalStampAmount (migration 0062), so every
-- existing invoice reads as "no discount" with no null-handling at any call
-- site.
--
-- The discount reduces the taxable base, so validateInvoiceTotals
-- (db/invoice_totals.go) and the auto-posting path (db/gl_posting.go) both
-- allocate it across the invoice's tax-rate groups proportionally to each
-- group's share of the subtotal.
ALTER TABLE invoices ADD COLUMN discountAmount INTEGER NOT NULL DEFAULT 0;

-- Whether printed documents show the total spelled out in words ("Arrêtée
-- la présente facture à la somme de ..."). A presentation toggle in the
-- organization's Formatting settings, alongside fiscalStampEnabled /
-- withholdingTaxEnabled (migration 0063) — frontend-only in the sense that
-- it gates a template placeholder's value, never a GL or validation rule.
ALTER TABLE organizations ADD COLUMN amountInWordsEnabled INTEGER NOT NULL DEFAULT 0;
