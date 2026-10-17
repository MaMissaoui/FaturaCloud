-- The five counter-numbered documents get the same rule as invoices
-- (migration 0106, audit F169): a number is unique within its organization.
-- Their Create* already skip a taken number when generating one and refuse a
-- taken explicit one (db/document_number_unique.go); this makes the database
-- refuse it too. Blank numbers are left out. Production had no duplicate in
-- any of the five when this was written (audit Phase 0, 2026-10-17).
CREATE UNIQUE INDEX orders_org_number_unique
  ON orders(organizationId, orderNumber) WHERE orderNumber <> '';
CREATE UNIQUE INDEX purchase_orders_org_number_unique
  ON purchase_orders(organizationId, orderNumber) WHERE orderNumber <> '';
CREATE UNIQUE INDEX outbound_deliveries_org_number_unique
  ON outbound_deliveries(organizationId, deliveryNumber) WHERE deliveryNumber <> '';
CREATE UNIQUE INDEX inbound_deliveries_org_number_unique
  ON inbound_deliveries(organizationId, deliveryNumber) WHERE deliveryNumber <> '';
CREATE UNIQUE INDEX production_orders_org_number_unique
  ON production_orders(organizationId, orderNumber) WHERE orderNumber <> '';
