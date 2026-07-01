package db

import (
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

type OutboundDelivery struct {
	ID              string  `db:"id"              json:"id"`
	OrganizationID  string  `db:"organizationId"  json:"organizationId"`
	OrderID         *string `db:"orderId"         json:"orderId"`
	DeliveryNumber  string  `db:"deliveryNumber"  json:"deliveryNumber"`
	DeliveryDate    int64   `db:"deliveryDate"    json:"deliveryDate"`
	ShippingAddress *string `db:"shippingAddress" json:"shippingAddress"`
	TrackingNumber  *string `db:"trackingNumber"  json:"trackingNumber"`
	Notes           *string `db:"notes"           json:"notes"`
	Status          string  `db:"status"          json:"status"`
	CreatedAt       int64   `db:"createdAt"       json:"createdAt"`
	// Joined
	OrderNumber *string `db:"orderNumber" json:"orderNumber"`
	ClientID    *string `db:"clientId"    json:"clientId"`
	ClientName  *string `db:"clientName"  json:"clientName"`
}

type OutboundDeliveryLineItem struct {
	ID              string  `db:"id"              json:"id"`
	DeliveryID      string  `db:"deliveryId"      json:"deliveryId"`
	OrderLineItemID *string `db:"orderLineItemId" json:"orderLineItemId"`
	ProductID       *string `db:"productId"       json:"productId"`
	Description     string  `db:"description"     json:"description"`
	Quantity        float64 `db:"quantity"        json:"quantity"`
	Unit            *string `db:"unit"            json:"unit"`
	Position        int     `db:"position"        json:"position"`
	// Joined from products via productId; nil when the line has no product
	// (free-text line) or the product isn't stock-tracked.
	StockEnabled   *int     `db:"stockEnabled"   json:"stockEnabled"`
	AvailableStock *float64 `db:"availableStock" json:"availableStock"`
}

type CreateDeliveryLineItemRequest struct {
	OrderLineItemID *string `json:"orderLineItemId"`
	ProductID       *string `json:"productId"`
	Description     string  `json:"description"`
	Quantity        float64 `json:"quantity"`
	Unit            *string `json:"unit"`
}

type CreateDeliveryRequest struct {
	ID              string                          `json:"id"`
	OrganizationID  string                          `json:"organizationId"`
	OrderID         *string                         `json:"orderId"`
	DeliveryNumber  string                          `json:"deliveryNumber"`
	DeliveryDate    int64                           `json:"deliveryDate"`
	ShippingAddress *string                         `json:"shippingAddress"`
	TrackingNumber  *string                         `json:"trackingNumber"`
	Notes           *string                         `json:"notes"`
	LineItems       []CreateDeliveryLineItemRequest `json:"lineItems"`
}

type UpdateDeliveryRequest struct {
	OrderID         *string                          `json:"orderId"`
	DeliveryNumber  *string                          `json:"deliveryNumber"`
	DeliveryDate    *int64                           `json:"deliveryDate"`
	ShippingAddress *string                          `json:"shippingAddress"`
	TrackingNumber  *string                          `json:"trackingNumber"`
	Notes           *string                          `json:"notes"`
	Status          *string                          `json:"status"`
	LineItems       *[]CreateDeliveryLineItemRequest `json:"lineItems"`
}

func (d *Database) GetDeliveries(organizationID string) ([]OutboundDelivery, error) {
	rows := []OutboundDelivery{}
	err := d.DB.Select(&rows, `
		SELECT od.*,
		       o.orderNumber,
		       o.clientId,
		       c.name AS clientName
		FROM outbound_deliveries od
		LEFT JOIN orders o ON od.orderId = o.id
		LEFT JOIN clients c ON o.clientId = c.id
		WHERE od.organizationId = ?
		ORDER BY od.deliveryDate DESC, od.createdAt DESC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_deliveries: %w", err)
	}
	return rows, nil
}

func (d *Database) GetDelivery(id string) (*OutboundDelivery, error) {
	var row OutboundDelivery
	err := d.DB.Get(&row, `
		SELECT od.*,
		       o.orderNumber,
		       o.clientId,
		       c.name AS clientName
		FROM outbound_deliveries od
		LEFT JOIN orders o ON od.orderId = o.id
		LEFT JOIN clients c ON o.clientId = c.id
		WHERE od.id = ?
		LIMIT 1`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("get_delivery: %w", err)
	}
	return &row, nil
}

func (d *Database) GetDeliveryLineItems(deliveryID string) ([]OutboundDeliveryLineItem, error) {
	items := []OutboundDeliveryLineItem{}
	err := d.DB.Select(&items, `
		SELECT dli.*,
		       p.stockEnabled AS stockEnabled,
		       p.stockQuantity AS availableStock
		FROM outbound_delivery_line_items dli
		LEFT JOIN products p ON dli.productId = p.id
		WHERE dli.deliveryId = ?
		ORDER BY dli.position ASC`,
		deliveryID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_delivery_line_items: %w", err)
	}
	return items, nil
}

func (d *Database) CreateDelivery(req CreateDeliveryRequest) (*OutboundDelivery, error) {
	_, err := d.DB.Exec(`
		INSERT INTO outbound_deliveries
		  (id, organizationId, orderId, deliveryNumber, deliveryDate, shippingAddress, trackingNumber, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.OrderID, req.DeliveryNumber,
		req.DeliveryDate, req.ShippingAddress, req.TrackingNumber, req.Notes,
	)
	if err != nil {
		return nil, fmt.Errorf("create_delivery: %w", err)
	}
	if err := d.replaceDeliveryLineItems(req.ID, req.LineItems); err != nil {
		return nil, err
	}
	return d.GetDelivery(req.ID)
}

func (d *Database) UpdateDelivery(id string, req UpdateDeliveryRequest) (*OutboundDelivery, error) {
	_, err := d.DB.Exec(`
		UPDATE outbound_deliveries SET
		  orderId         = COALESCE(?, orderId),
		  deliveryNumber  = COALESCE(?, deliveryNumber),
		  deliveryDate    = COALESCE(?, deliveryDate),
		  shippingAddress = COALESCE(?, shippingAddress),
		  trackingNumber  = COALESCE(?, trackingNumber),
		  notes           = COALESCE(?, notes),
		  status          = COALESCE(?, status)
		WHERE id = ?`,
		req.OrderID, req.DeliveryNumber, req.DeliveryDate,
		req.ShippingAddress, req.TrackingNumber, req.Notes, req.Status,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("update_delivery: %w", err)
	}
	if req.LineItems != nil {
		if err := d.replaceDeliveryLineItems(id, *req.LineItems); err != nil {
			return nil, err
		}
	}
	return d.GetDelivery(id)
}

// deliveryStockLine is a delivery line item resolved to its stock-enabled product.
type deliveryStockLine struct {
	ProductID      string  `db:"productId"`
	ProductName    string  `db:"productName"`
	Quantity       float64 `db:"quantity"`
	AvailableStock float64 `db:"availableStock"`
}

// getShippableStockLines returns the delivery's line items that are linked to a
// stock-enabled product (whether picked directly or copied from an order line
// item) — the only lines that affect inventory.
func getShippableStockLines(tx *sqlx.Tx, deliveryID string) ([]deliveryStockLine, error) {
	lines := []deliveryStockLine{}
	err := tx.Select(&lines, `
		SELECT p.id AS productId,
		       p.name AS productName,
		       dli.quantity AS quantity,
		       p.stockQuantity AS availableStock
		FROM outbound_delivery_line_items dli
		JOIN products p ON dli.productId = p.id
		WHERE dli.deliveryId = ? AND p.stockEnabled = 1`,
		deliveryID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_shippable_stock_lines: %w", err)
	}
	return lines, nil
}

// UpdateDeliveryStatus updates a delivery's status, reducing inventory when a
// draft delivery is marked shipped (rejecting the transition if any stock-enabled
// product doesn't have enough available stock) and restoring inventory when an
// already-shipped delivery is cancelled.
func (d *Database) UpdateDeliveryStatus(id, status string) (*OutboundDelivery, error) {
	current, err := d.GetDelivery(id)
	if err != nil {
		return nil, fmt.Errorf("update_delivery_status lookup: %w", err)
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_delivery_status begin: %w", err)
	}
	defer tx.Rollback()

	switch {
	case current.Status == "draft" && status == "shipped":
		lines, err := getShippableStockLines(tx, id)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			if line.Quantity > line.AvailableStock {
				return nil, fmt.Errorf(
					"insufficient stock for %q: available %.2f, requested %.2f",
					line.ProductName, line.AvailableStock, line.Quantity,
				)
			}
		}
		for _, line := range lines {
			movementID, _ := gonanoid.New()
			if err := insertStockMovementTx(tx, CreateStockMovementRequest{
				ID:             movementID,
				OrganizationID: current.OrganizationID,
				ProductID:      line.ProductID,
				Type:           "out",
				Quantity:       -line.Quantity,
				Note:           ptrStr("Delivery " + current.DeliveryNumber),
				Reference:      &current.DeliveryNumber,
			}); err != nil {
				return nil, fmt.Errorf("update_delivery_status reduce_stock: %w", err)
			}
		}

	case current.Status == "shipped" && status == "cancelled":
		lines, err := getShippableStockLines(tx, id)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			movementID, _ := gonanoid.New()
			if err := insertStockMovementTx(tx, CreateStockMovementRequest{
				ID:             movementID,
				OrganizationID: current.OrganizationID,
				ProductID:      line.ProductID,
				Type:           "in",
				Quantity:       line.Quantity,
				Note:           ptrStr("Delivery " + current.DeliveryNumber + " cancelled"),
				Reference:      &current.DeliveryNumber,
			}); err != nil {
				return nil, fmt.Errorf("update_delivery_status restore_stock: %w", err)
			}
		}
	}

	if _, err := tx.Exec(`UPDATE outbound_deliveries SET status = ? WHERE id = ?`, status, id); err != nil {
		return nil, fmt.Errorf("update_delivery_status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_delivery_status commit: %w", err)
	}
	return d.GetDelivery(id)
}

func ptrStr(s string) *string { return &s }

func (d *Database) DeleteDelivery(id string) (bool, error) {
	current, err := d.GetDelivery(id)
	if err != nil {
		return false, fmt.Errorf("delete_delivery lookup: %w", err)
	}
	if current.Status == "shipped" || current.Status == "delivered" {
		return false, fmt.Errorf("cannot delete a %s delivery — cancel it instead", current.Status)
	}

	res, err := d.DB.Exec(`DELETE FROM outbound_deliveries WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete_delivery: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (d *Database) replaceDeliveryLineItems(deliveryID string, items []CreateDeliveryLineItemRequest) error {
	_, err := d.DB.Exec(`DELETE FROM outbound_delivery_line_items WHERE deliveryId = ?`, deliveryID)
	if err != nil {
		return fmt.Errorf("delete_delivery_line_items: %w", err)
	}
	for i, item := range items {
		id, _ := gonanoid.New()
		productID := item.ProductID
		if productID == nil && item.OrderLineItemID != nil {
			var resolved sql.NullString
			if err := d.DB.Get(&resolved, `SELECT productId FROM orderLineItems WHERE id = ?`, *item.OrderLineItemID); err == nil && resolved.Valid {
				productID = &resolved.String
			}
		}
		_, err := d.DB.Exec(`
			INSERT INTO outbound_delivery_line_items
			  (id, deliveryId, orderLineItemId, productId, description, quantity, unit, position)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, deliveryID, item.OrderLineItemID, productID, item.Description, item.Quantity, item.Unit, i,
		)
		if err != nil {
			return fmt.Errorf("insert_delivery_line_item: %w", err)
		}
	}
	return nil
}

func (d *Database) NextDeliveryNumber(organizationID string) string {
	var count int
	_ = d.DB.Get(&count, `SELECT COUNT(*) FROM outbound_deliveries WHERE organizationId = ?`, organizationID)
	return fmt.Sprintf("DEL-%04d", count+1)
}
