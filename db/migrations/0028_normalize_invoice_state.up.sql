-- Normalize invoice.state onto the canonical set (draft|sent|paid|cancelled).
-- The frontend previously offered "void"; the canonical name is "cancelled"
-- (see CLAUDE.md and existing data). Any other stray value collapses to draft.
UPDATE invoices SET state = 'cancelled' WHERE state = 'void';
UPDATE invoices SET state = 'draft' WHERE state NOT IN ('draft', 'sent', 'paid', 'cancelled');
