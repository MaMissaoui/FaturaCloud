-- Neither the 'custom' -> NULL normalization nor the Tunisia backfill in the
-- up migration is reversed: invoiceLayout has no effect before 0089, so the
-- values are behaviorally identical either way.
ALTER TABLE organizations RENAME COLUMN documentLayout TO invoiceLayout;
