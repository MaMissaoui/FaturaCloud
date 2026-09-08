-- A standalone outbound delivery (no linked order) had no way to record who
-- it's for — ClientID on the API struct was only ever a value joined from
-- the linked order. ON DELETE SET NULL mirrors orders.clientId's own
-- precedent (migration 0022): deleting a client just detaches a delivery
-- from it rather than blocking the delete or cascading the delivery away.
ALTER TABLE outbound_deliveries ADD COLUMN clientId TEXT REFERENCES clients(id) ON DELETE SET NULL;
