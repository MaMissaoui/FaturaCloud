package db

import (
	"fmt"

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
	Description     string  `db:"description"     json:"description"`
	Quantity        float64 `db:"quantity"        json:"quantity"`
	Unit            *string `db:"unit"            json:"unit"`
	Position        int     `db:"position"        json:"position"`
}

type CreateDeliveryLineItemRequest struct {
	OrderLineItemID *string `json:"orderLineItemId"`
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
	err := d.DB.Select(&items,
		`SELECT * FROM outbound_delivery_line_items WHERE deliveryId = ? ORDER BY position ASC`,
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

func (d *Database) UpdateDeliveryStatus(id, status string) (*OutboundDelivery, error) {
	_, err := d.DB.Exec(`UPDATE outbound_deliveries SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return nil, fmt.Errorf("update_delivery_status: %w", err)
	}
	return d.GetDelivery(id)
}

func (d *Database) DeleteDelivery(id string) (bool, error) {
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
		_, err := d.DB.Exec(`
			INSERT INTO outbound_delivery_line_items
			  (id, deliveryId, orderLineItemId, description, quantity, unit, position)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, deliveryID, item.OrderLineItemID, item.Description, item.Quantity, item.Unit, i,
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
