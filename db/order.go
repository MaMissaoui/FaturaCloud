package db

import (
	"database/sql"
	"errors"
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

type Order struct {
	ID              string  `db:"id"              json:"id"`
	OrganizationID  string  `db:"organizationId"  json:"organizationId"`
	ClientID        *string `db:"clientId"        json:"clientId"`
	OrderNumber     string  `db:"orderNumber"     json:"orderNumber"`
	Status          string  `db:"status"          json:"status"`
	OrderDate       int64   `db:"orderDate"       json:"orderDate"`
	DeliveryDate    *int64  `db:"deliveryDate"    json:"deliveryDate"`
	ShippingAddress *string `db:"shippingAddress" json:"shippingAddress"`
	TrackingNumber  *string `db:"trackingNumber"  json:"trackingNumber"`
	Notes           *string `db:"notes"           json:"notes"`
	ClientName      *string `db:"clientName"      json:"clientName"`
	CreatedAt       string  `db:"createdAt"       json:"createdAt"`
}

type OrderLineItem struct {
	ID          string  `db:"id"          json:"id"`
	OrderID     string  `db:"orderId"     json:"orderId"`
	ProductID   *string `db:"productId"   json:"productId"`
	Description string  `db:"description" json:"description"`
	Quantity    float64 `db:"quantity"    json:"quantity"`
	UnitPrice   int64   `db:"unitPrice"   json:"unitPrice"`
	Position    int     `db:"position"    json:"position"`
}

type CreateOrderLineItemRequest struct {
	ProductID   *string `json:"productId"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice"` // cents sent from frontend
}

type CreateOrderRequest struct {
	ID              string                       `json:"id"`
	OrganizationID  string                       `json:"organizationId"`
	ClientID        *string                      `json:"clientId"`
	OrderNumber     string                       `json:"orderNumber"`
	Status          string                       `json:"status"`
	OrderDate       int64                        `json:"orderDate"`
	DeliveryDate    *int64                       `json:"deliveryDate"`
	ShippingAddress *string                      `json:"shippingAddress"`
	TrackingNumber  *string                      `json:"trackingNumber"`
	Notes           *string                      `json:"notes"`
	LineItems       []CreateOrderLineItemRequest `json:"lineItems"`
}

type UpdateOrderRequest struct {
	ClientID        *string                       `json:"clientId"`
	OrderNumber     *string                       `json:"orderNumber"`
	OrderDate       *int64                        `json:"orderDate"`
	DeliveryDate    *int64                        `json:"deliveryDate"`
	ShippingAddress *string                       `json:"shippingAddress"`
	TrackingNumber  *string                       `json:"trackingNumber"`
	Notes           *string                       `json:"notes"`
	LineItems       *[]CreateOrderLineItemRequest `json:"lineItems"`
}

// validOrderStatuses are the only values orders.status may take (see
// CLAUDE.md); orderStatusTransitions below governs which moves between them
// are legal once an order exists.
var validOrderStatuses = map[string]bool{
	"draft": true, "confirmed": true, "shipped": true, "delivered": true, "cancelled": true,
}

// orderStatusTransitions enumerates the only legal order status moves;
// "delivered" and "cancelled" are terminal (absent as keys, so any move out
// of them is rejected). Mirrors src/routes/orders/details.tsx's
// STATUS_TRANSITIONS, enforced here too since that's client-side only.
var orderStatusTransitions = map[string]map[string]bool{
	"draft":     {"confirmed": true, "cancelled": true},
	"confirmed": {"shipped": true, "cancelled": true},
	"shipped":   {"delivered": true, "cancelled": true},
}

func (d *Database) GetOrders(organizationID string) ([]Order, error) {
	orders := []Order{}
	err := d.DB.Select(&orders, `
		SELECT orders.*, clients.name AS clientName
		FROM orders
		LEFT JOIN clients ON orders.clientId = clients.id
		WHERE orders.organizationId = ?
		ORDER BY orders.orderDate DESC, orders.createdAt DESC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_orders: %w", err)
	}
	return orders, nil
}

func (d *Database) GetOrder(orderID string) (*Order, error) {
	var order Order
	err := d.DB.Get(&order, `
		SELECT orders.*, clients.name AS clientName
		FROM orders
		LEFT JOIN clients ON orders.clientId = clients.id
		WHERE orders.id = ?
		LIMIT 1`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_order: %w", err)
	}
	return &order, nil
}

func (d *Database) GetOrderLineItems(orderID string) ([]OrderLineItem, error) {
	items := []OrderLineItem{}
	err := d.DB.Select(&items,
		`SELECT * FROM orderLineItems WHERE orderId = ? ORDER BY position ASC`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_order_line_items: %w", err)
	}
	return items, nil
}

// GetOrderDeliveredQuantities sums delivered quantity per orderLineItemId across
// all non-cancelled deliveries for the given order — used to compute how much
// of each line is still outstanding.
func (d *Database) GetOrderDeliveredQuantities(orderID string) (map[string]float64, error) {
	rows := []struct {
		OrderLineItemID string  `db:"orderLineItemId"`
		Delivered       float64 `db:"delivered"`
	}{}
	err := d.DB.Select(&rows, `
		SELECT dli.orderLineItemId AS orderLineItemId, SUM(dli.quantity) AS delivered
		FROM outbound_delivery_line_items dli
		JOIN outbound_deliveries od ON dli.deliveryId = od.id
		WHERE od.orderId = ? AND od.status != 'cancelled' AND dli.orderLineItemId IS NOT NULL
		GROUP BY dli.orderLineItemId`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_order_delivered_quantities: %w", err)
	}

	result := make(map[string]float64, len(rows))
	for _, row := range rows {
		result[row.OrderLineItemID] = row.Delivered
	}
	return result, nil
}

func (d *Database) CreateOrder(req CreateOrderRequest) (*Order, error) {
	if req.Status == "" {
		req.Status = "draft"
	}
	if !validOrderStatuses[req.Status] {
		return nil, newValidationError("invalid order status %q", req.Status)
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_order begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		INSERT INTO orders (id, organizationId, clientId, orderNumber, status, orderDate, deliveryDate, shippingAddress, trackingNumber, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.ClientID, req.OrderNumber, req.Status,
		req.OrderDate, req.DeliveryDate, req.ShippingAddress, req.TrackingNumber, req.Notes,
	)
	if err != nil {
		return nil, fmt.Errorf("create_order insert: %w", err)
	}

	for i, item := range req.LineItems {
		itemID, _ := gonanoid.New()
		_, err = tx.Exec(`
			INSERT INTO orderLineItems (id, orderId, productId, description, quantity, unitPrice, position)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			itemID, req.ID, item.ProductID, item.Description, item.Quantity, int64(item.UnitPrice), i,
		)
		if err != nil {
			return nil, fmt.Errorf("create_order line_item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_order commit: %w", err)
	}

	return d.GetOrder(req.ID)
}

func (d *Database) UpdateOrder(orderID string, updates UpdateOrderRequest) (*Order, error) {
	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_order begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		UPDATE orders
		SET clientId        = ?,
		    orderNumber     = COALESCE(?, orderNumber),
		    orderDate       = COALESCE(?, orderDate),
		    deliveryDate    = ?,
		    shippingAddress = ?,
		    trackingNumber  = ?,
		    notes           = ?
		WHERE id = ?`,
		updates.ClientID,
		updates.OrderNumber, updates.OrderDate,
		updates.DeliveryDate, updates.ShippingAddress, updates.TrackingNumber, updates.Notes,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("update_order exec: %w", err)
	}

	if updates.LineItems != nil {
		if _, err = tx.Exec(`DELETE FROM orderLineItems WHERE orderId = ?`, orderID); err != nil {
			return nil, fmt.Errorf("update_order delete_items: %w", err)
		}
		for i, item := range *updates.LineItems {
			itemID, _ := gonanoid.New()
			_, err = tx.Exec(`
				INSERT INTO orderLineItems (id, orderId, productId, description, quantity, unitPrice, position)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				itemID, orderID, item.ProductID, item.Description, item.Quantity, int64(item.UnitPrice), i,
			)
			if err != nil {
				return nil, fmt.Errorf("update_order line_item: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_order commit: %w", err)
	}

	return d.GetOrder(orderID)
}

// UpdateOrderStatus updates an order's status. Any transition not in
// orderStatusTransitions (including out of a terminal "delivered"/"cancelled"
// state) is rejected; setting an order to its current status is a no-op.
func (d *Database) UpdateOrderStatus(orderID string, status string) (*Order, error) {
	current, err := d.GetOrder(orderID)
	if err != nil {
		return nil, fmt.Errorf("update_order_status lookup: %w", err)
	}
	if status != current.Status && !orderStatusTransitions[current.Status][status] {
		return nil, newValidationError("cannot transition order from %q to %q", current.Status, status)
	}

	_, err = d.DB.Exec(`UPDATE orders SET status = ? WHERE id = ?`, status, orderID)
	if err != nil {
		return nil, fmt.Errorf("update_order_status: %w", err)
	}
	return d.GetOrder(orderID)
}

func (d *Database) DeleteOrder(orderID string) (bool, error) {
	current, err := d.GetOrder(orderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if current.Status == "shipped" || current.Status == "delivered" {
		return false, newValidationError("cannot delete a %s order — cancel it instead", current.Status)
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return false, fmt.Errorf("delete_order begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err = tx.Exec(`DELETE FROM orderLineItems WHERE orderId = ?`, orderID); err != nil {
		return false, fmt.Errorf("delete_order items: %w", err)
	}

	res, err := tx.Exec(`DELETE FROM orders WHERE id = ?`, orderID)
	if err != nil {
		return false, fmt.Errorf("delete_order: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("delete_order commit: %w", err)
	}

	n, _ := res.RowsAffected()
	return n > 0, nil
}
