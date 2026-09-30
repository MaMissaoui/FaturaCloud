-- Loans migrated from a paper loan register (docs/loan-register-migration-plan.md,
-- db/opening_loan.go). invoices.origin = 'opening' marks a receivable brought
-- forward at cutover, not a sale made in the app: it never moves stock, posts
-- no revenue or VAT, and is frozen — only Cash Book collections change it
-- (and, from phase 2, only a batch undo removes it). NULL is an ordinary
-- invoice. importBatchId groups the invoices one import created.
ALTER TABLE invoices ADD COLUMN origin TEXT;
ALTER TABLE invoices ADD COLUMN importBatchId TEXT;
CREATE INDEX IF NOT EXISTS invoices_importBatchId ON invoices(importBatchId);

-- payments.origin = 'opening' marks a migrated loan's "paid to date": a
-- record of money received before the cutover, with no GL entry and no
-- register (its bankAccountId is the organization's Retained Earnings
-- account). A separate column rather than a new payments.method value,
-- because method has a CHECK constraint SQLite can only change by
-- rebuilding the table.
ALTER TABLE payments ADD COLUMN origin TEXT;
