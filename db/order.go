package db

import (
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
	Status          *string                       `json:"status"`
	OrderDate       *int64                        `json:"orderDate"`
	DeliveryDate    *int64                        `json:"deliveryDate"`
	ShippingAddress *string                       `json:"shippingAddress"`
	TrackingNumber  *string                       `json:"trackingNumber"`
	Notes           *string                       `json:"notes"`
	LineItems       *[]CreateOrderLineItemRequest `json:"lineItems"`
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

func (d *Database) CreateOrder(req CreateOrderRequest) (*Order, error) {
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
		    status          = COALESCE(?, status),
		    orderDate       = COALESCE(?, orderDate),
		    deliveryDate    = ?,
		    shippingAddress = ?,
		    trackingNumber  = ?,
		    notes           = ?
		WHERE id = ?`,
		updates.ClientID,
		updates.OrderNumber, updates.Status, updates.OrderDate,
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

func (d *Database) UpdateOrderStatus(orderID string, status string) (*Order, error) {
	_, err := d.DB.Exec(`UPDATE orders SET status = ? WHERE id = ?`, status, orderID)
	if err != nil {
		return nil, fmt.Errorf("update_order_status: %w", err)
	}
	return d.GetOrder(orderID)
}

func (d *Database) DeleteOrder(orderID string) (bool, error) {
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
