-- How the organization values its stock (db/inventory_valuation.go):
-- 'perpetual' (NULL/''/anything unrecognized — today's behaviour) posts GRNI
-- on receipt, COGS on shipment and GL traces for manual adjustments and
-- production, and refuses a stock-out with no cost basis; 'quantity_only'
-- tracks stock by quantity alone — none of those entries post, no unit cost
-- is ever required, and bills for stock-enabled products are expensed (the
-- periodic method a small business whose accountant values closing stock at
-- year end uses). No backfill: every existing organization stays perpetual.
ALTER TABLE organizations ADD COLUMN inventoryValuation TEXT;
