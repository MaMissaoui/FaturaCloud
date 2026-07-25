-- BT-10 buyer reference (e.g. a German Leitweg-ID) — mandatory for
-- XRechnung/B2G. BT-20 payment terms free text; BT-9 due date is already
-- covered by invoices.dueDate.
ALTER TABLE invoices ADD COLUMN buyerReference TEXT;
ALTER TABLE invoices ADD COLUMN paymentTerms TEXT;
