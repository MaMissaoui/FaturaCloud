-- F58: CloseFiscalYear's activity snapshot, both GL exports
-- (export_fec.go/export_datev.go), and the report group-bys all filter
-- journal_entries by fiscalYearId with no supporting index — a full scan of
-- a table that's append-mostly and never compacted, growing without bound.
--
-- Numbered 0060, not 0059, on the assumption PR #96 (F48, the partial
-- unique index on journal_entries) merges first. If these land out of
-- order, renumber before merging.
CREATE INDEX idx_journal_entries_fiscal_year_id ON journal_entries(fiscalYearId);
