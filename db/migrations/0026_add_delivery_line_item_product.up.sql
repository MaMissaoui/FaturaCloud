ALTER TABLE outbound_delivery_line_items ADD COLUMN productId TEXT REFERENCES products(id) ON DELETE SET NULL;

UPDATE outbound_delivery_line_items
SET productId = (
  SELECT oli.productId
  FROM orderLineItems oli
  WHERE oli.id = outbound_delivery_line_items.orderLineItemId
)
WHERE orderLineItemId IS NOT NULL;
