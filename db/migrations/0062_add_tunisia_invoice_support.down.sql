ALTER TABLE invoices DROP COLUMN fiscalStampAmount;
ALTER TABLE invoices DROP COLUMN withholdingTaxRate;
ALTER TABLE invoices DROP COLUMN withholdingTaxAmount;
ALTER TABLE organizations DROP COLUMN defaultFiscalStampAmount;
ALTER TABLE organizations DROP COLUMN defaultStampDutyAccountId;
ALTER TABLE organizations DROP COLUMN invoiceLayout;
