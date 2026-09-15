-- Snapshot componentSku/componentUnit onto production order component lines
-- (audit 2026-09-14 F75).
--
-- 0075 denormalized only componentName and left componentSku/componentUnit
-- to a LEFT JOIN on products, so both went NULL once the component product
-- was deleted while componentName survived — a half-frozen snapshot. That
-- contradicts 0075's own comment, which claims it follows "the same shape
-- bill_of_materials_version_lines already uses"; 0074 snapshots all four
-- columns for exactly this reason.
ALTER TABLE production_order_component_lines ADD COLUMN componentSku TEXT;
ALTER TABLE production_order_component_lines ADD COLUMN componentUnit TEXT;

-- Backfill from the still-linked products. A line whose component was
-- already deleted stays NULL, which is the same thing the LEFT JOIN was
-- already returning for it — no information is lost, and none is invented.
UPDATE production_order_component_lines
SET componentSku = (
      SELECT p.sku FROM products p WHERE p.id = production_order_component_lines.componentProductId
    ),
    componentUnit = (
      SELECT p.unit FROM products p WHERE p.id = production_order_component_lines.componentProductId
    )
WHERE componentProductId IS NOT NULL;
