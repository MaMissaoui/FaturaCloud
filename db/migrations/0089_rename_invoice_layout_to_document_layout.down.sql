-- The 'custom' -> NULL normalization in the up migration isn't reversed:
-- 'custom' had no effect, so NULL is behaviorally identical.
ALTER TABLE organizations RENAME COLUMN documentLayout TO invoiceLayout;
