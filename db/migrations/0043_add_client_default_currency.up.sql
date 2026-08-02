-- Mirrors vendors.defaultCurrency (migration 0029) so clients have the same
-- "usual currency for this party" convenience field vendors already have.
-- Purely informational — it prefills a new invoice's currency, nothing more.
ALTER TABLE clients ADD COLUMN defaultCurrency TEXT;
