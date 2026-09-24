-- Line-level loan settlement from the Cash Book. A payment application may
-- now name the invoice line it settles, so a cashier collecting the balance
-- for one item of a loan sale (one appliance out of several) pays that line
-- rather than an amount spread across the whole invoice.
--
-- NULL (every pre-existing row, and every payment recorded through the
-- invoice-level payment flow) keeps meaning "applies to the invoice as a
-- whole": GetLoanStatus spreads those across the invoice's lines, while a
-- line-targeted application counts against its own line only. The GL is
-- unaffected either way — AR is still settled per invoice.
--
-- ON DELETE SET NULL: invoice line items are deleted and reinserted when an
-- invoice's lines are edited (only possible once it has no posted GL entry,
-- i.e. after being moved back to draft). The application then falls back to
-- invoice-level, which is still the correct amount against the invoice.
ALTER TABLE payment_applications ADD COLUMN invoiceLineItemId TEXT
    REFERENCES invoiceLineItems(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS payment_applications_invoice_line_item
    ON payment_applications(invoiceLineItemId);
