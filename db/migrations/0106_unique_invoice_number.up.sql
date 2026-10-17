-- An invoice number is unique within its organization (audit F169).
-- CreateInvoice/UpdateInvoice already refuse a taken number
-- (db/invoice_number.go); this makes the database refuse it too. Migrated
-- opening loans keep their paper register references, unique among
-- themselves, so they are left out, as are blank numbers. Production had no
-- duplicate and no blank number when this was written (audit Phase 0,
-- 2026-10-17).
CREATE UNIQUE INDEX invoices_org_number_unique
  ON invoices(organizationId, number)
  WHERE number <> '' AND COALESCE(origin, '') <> 'opening';
