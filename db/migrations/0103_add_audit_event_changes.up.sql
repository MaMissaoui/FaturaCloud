-- What a change changed (db/audit_changes.go): a JSON array of
-- {"field","from","to"} for each column of the document that differs after
-- the change, bank accounts masked ("masked": true) and secrets never
-- included. '' for rows recorded before this column, creates and deletions.
ALTER TABLE audit_events ADD COLUMN changes TEXT NOT NULL DEFAULT '';
