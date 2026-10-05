-- One-time catch-up for the rule payments now enforce (db/payment_state.go,
-- owner decision 2026-10-05): a sent invoice or an approved bill whose
-- non-voided payments cover its whole total is "paid". Documents settled
-- before the rule kept their manual state; this moves them, for every
-- organization. A pure state change — "sent"/"approved" and "paid" carry the
-- same GL and stock presence, so nothing is posted or reversed.
UPDATE invoices SET state = 'paid'
WHERE state = 'sent' AND total > 0 AND total <= (
    SELECT COALESCE(SUM(pa.amount), 0)
    FROM payment_applications pa
    JOIN payments p ON p.id = pa.paymentId
    WHERE pa.documentType = 'invoice' AND pa.documentId = invoices.id
      AND p.status != 'voided'
);

UPDATE incoming_invoices SET state = 'paid'
WHERE state = 'approved' AND total > 0 AND total <= (
    SELECT COALESCE(SUM(pa.amount), 0)
    FROM payment_applications pa
    JOIN payments p ON p.id = pa.paymentId
    WHERE pa.documentType = 'incoming_invoice' AND pa.documentId = incoming_invoices.id
      AND p.status != 'voided'
);
