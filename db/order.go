package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

type Order struct {
	ID             string  `db:"id"              json:"id"`
	OrganizationID string  `db:"organizationId"  json:"organizationId"`
	ClientID       *string `db:"clientId"        json:"clientId"`
	OrderNumber    string  `db:"orderNumber"     json:"orderNumber"`
	Status         string  `db:"status"          json:"status"`
	OrderDate      int64   `db:"orderDate"       json:"orderDate"`
	DeliveryDate   *int64  `db:"deliveryDate"    json:"deliveryDate"`
	// Nullable, like purchase_orders.currency — null means "the
	// organization's own currency".
	Currency *string `db:"currency" json:"currency"`
	// See db/exchange_rate.go for the rate direction convention.
	ExchangeRate     *string `db:"exchangeRate"     json:"exchangeRate"`
	ExchangeRateDate *int64  `db:"exchangeRateDate" json:"exchangeRateDate"`
	ShippingAddress  *string `db:"shippingAddress" json:"shippingAddress"`
	TrackingNumber   *string `db:"trackingNumber"  json:"trackingNumber"`
	Notes            *string `db:"notes"           json:"notes"`
	ClientName       *string `db:"clientName"      json:"clientName"`
	CreatedAt        string  `db:"createdAt"       json:"createdAt"`
}

type OrderLineItem struct {
	ID          string  `db:"id"          json:"id"`
	OrderID     string  `db:"orderId"     json:"orderId"`
	ProductID   *string `db:"productId"   json:"productId"`
	Description string  `db:"description" json:"description"`
	Quantity    float64 `db:"quantity"    json:"quantity"`
	UnitPrice   int64   `db:"unitPrice"   json:"unitPrice"`
	Position    int     `db:"position"    json:"position"`
	// Joined from products via productId; nil on a free-text line or an
	// unset SKU — same convention as OutboundDeliveryLineItem.SKU.
	SKU *string `db:"sku" json:"sku"`
}

type CreateOrderLineItemRequest struct {
	// ID is the existing orderLineItems row this line should keep, echoed
	// back by the client from a previous read. Empty/absent means a newly
	// added line. Preserving it is what keeps
	// outbound_delivery_line_items.orderLineItemId intact across an edit —
	// see db/line_item_reconcile.go (F70).
	ID          *string `json:"id"`
	ProductID   *string `json:"productId"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice"` // cents sent from frontend
}

type CreateOrderRequest struct {
	ID               string                       `json:"id"`
	OrganizationID   string                       `json:"organizationId"`
	ClientID         *string                      `json:"clientId"`
	OrderNumber      string                       `json:"orderNumber"`
	Status           string                       `json:"status"`
	OrderDate        int64                        `json:"orderDate"`
	DeliveryDate     *int64                       `json:"deliveryDate"`
	Currency         *string                      `json:"currency"`
	ExchangeRate     *float64                     `json:"exchangeRate"`
	ExchangeRateDate *int64                       `json:"exchangeRateDate"`
	ShippingAddress  *string                      `json:"shippingAddress"`
	TrackingNumber   *string                      `json:"trackingNumber"`
	Notes            *string                      `json:"notes"`
	LineItems        []CreateOrderLineItemRequest `json:"lineItems"`
}

type UpdateOrderRequest struct {
	ClientID         *string                       `json:"clientId"`
	OrderNumber      *string                       `json:"orderNumber"`
	OrderDate        *int64                        `json:"orderDate"`
	DeliveryDate     *int64                        `json:"deliveryDate"`
	Currency         *string                       `json:"currency"`
	ExchangeRate     *float64                      `json:"exchangeRate"`
	ExchangeRateDate *int64                        `json:"exchangeRateDate"`
	ShippingAddress  *string                       `json:"shippingAddress"`
	TrackingNumber   *string                       `json:"trackingNumber"`
	Notes            *string                       `json:"notes"`
	LineItems        *[]CreateOrderLineItemRequest `json:"lineItems"`
}

// validOrderStatuses are the only values orders.status may take (see
// CLAUDE.md); orderStatusTransitions below governs which moves between them
// are legal once an order exists.
var validOrderStatuses = map[string]bool{
	"draft": true, "confirmed": true, "shipped": true, "delivered": true, "cancelled": true,
}

// orderStatusTransitions enumerates the only legal order status moves;
// "delivered" is terminal (absent as a key, so any move out of it is
// rejected). "cancelled" falls back to "confirmed"/"shipped"/"delivered" —
// safe unconditionally since order status changes have zero side effects
// (no stock, no GL), same reasoning as purchaseOrderStatusTransitions in
// db/purchase_order.go. Mirrors orderStatusTransitionMatrix in
// src/types/order.ts, enforced here too since that's client-side only.
var orderStatusTransitions = map[string]map[string]bool{
	"draft":     {"confirmed": true, "cancelled": true},
	"confirmed": {"shipped": true, "cancelled": true},
	"shipped":   {"delivered": true, "cancelled": true},
	"cancelled": {"confirmed": true, "shipped": true, "delivered": true},
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
		`SELECT oli.*, p.sku AS sku
		 FROM orderLineItems oli
		 LEFT JOIN products p ON oli.productId = p.id
		 WHERE oli.orderId = ? ORDER BY oli.position ASC`,
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

// advanceOrderStatusToShippedTx bumps a linked order from "confirmed" to
// "shipped" the moment the first of its outbound deliveries actually ships —
// called from UpdateDeliveryStatus inside its own transaction (never a
// separate one; SetMaxOpenConns(1) forbids a second connection while one is
// already open). The `WHERE status = 'confirmed'` guard makes this
// deliberately best-effort: an order already shipped/delivered, one a user
// left in draft, or one that was cancelled has nothing to advance, and that
// is not an error — this is a convenience cascade automating what the
// "Mark as shipped" button already does manually, never a hard requirement
// the delivery's own status change should fail over.
func advanceOrderStatusToShippedTx(tx sqlExecer, orderID string) error {
	if _, err := tx.Exec(
		`UPDATE orders SET status = 'shipped' WHERE id = ? AND status = 'confirmed'`, orderID,
	); err != nil {
		return fmt.Errorf("advance_order_status_shipped: %w", err)
	}
	return nil
}

// orderFullyDeliveredTx reports whether every line item on the order is
// covered by outbound deliveries that have actually reached "delivered" —
// deliberately stricter than GetOrderDeliveredQuantities above (which also
// counts draft/shipped deliveries, the right definition for the order page's
// in-progress "x / y delivered" badge, the wrong one for deciding the order
// itself is complete). An order with no line items is never "fully
// delivered" — there is nothing to deliver, and treating that as complete
// would auto-advance an order nobody has finished building yet.
func orderFullyDeliveredTx(tx sqlSelectExecer, orderID string) (bool, error) {
	var lines []struct {
		ID       string  `db:"id"`
		Quantity float64 `db:"quantity"`
	}
	if err := tx.Select(&lines, `SELECT id, quantity FROM orderLineItems WHERE orderId = ?`, orderID); err != nil {
		return false, fmt.Errorf("order_fully_delivered lines: %w", err)
	}
	if len(lines) == 0 {
		return false, nil
	}

	rows := []struct {
		OrderLineItemID string  `db:"orderLineItemId"`
		Delivered       float64 `db:"delivered"`
	}{}
	if err := tx.Select(&rows, `
		SELECT dli.orderLineItemId AS orderLineItemId, SUM(dli.quantity) AS delivered
		FROM outbound_delivery_line_items dli
		JOIN outbound_deliveries od ON dli.deliveryId = od.id
		WHERE od.orderId = ? AND od.status = 'delivered' AND dli.orderLineItemId IS NOT NULL
		GROUP BY dli.orderLineItemId`,
		orderID,
	); err != nil {
		return false, fmt.Errorf("order_fully_delivered delivered: %w", err)
	}
	delivered := make(map[string]float64, len(rows))
	for _, r := range rows {
		delivered[r.OrderLineItemID] = r.Delivered
	}

	// Same REAL-column tolerance precedent as products.stockQuantity
	// elsewhere in this codebase.
	const epsilon = 1e-9
	for _, line := range lines {
		if delivered[line.ID]+epsilon < line.Quantity {
			return false, nil
		}
	}
	return true, nil
}

// advanceOrderStatusToDeliveredTx bumps a linked order from "shipped" to
// "delivered" once every one of its lines is fully covered by delivered (not
// merely shipped) deliveries — same best-effort convention as
// advanceOrderStatusToShippedTx above.
func advanceOrderStatusToDeliveredTx(tx sqlSelectExecer, orderID string) error {
	fullyDelivered, err := orderFullyDeliveredTx(tx, orderID)
	if err != nil {
		return err
	}
	if !fullyDelivered {
		return nil
	}
	if _, err := tx.Exec(
		`UPDATE orders SET status = 'delivered' WHERE id = ? AND status = 'shipped'`, orderID,
	); err != nil {
		return fmt.Errorf("advance_order_status_delivered: %w", err)
	}
	return nil
}

// checkOrderFKOwnership validates that clientId (if set) and each line
// item's productId belong to the SAME organization as the order (issue
// #189) — Phase C's route-level membership check only proves the caller
// belongs to organizationID, not that ids referenced INSIDE the body do
// too. clientID nil/empty and an empty lineItems both mean "not part of
// this request," not a mismatch.
func (d *Database) checkOrderFKOwnership(organizationID string, clientID *string, lineItems []CreateOrderLineItemRequest) error {
	if clientID != nil && *clientID != "" {
		client, err := d.GetClient(*clientID)
		if err != nil {
			return newValidationError("client not found")
		}
		if err := requireSameOrg(organizationID, client.OrganizationID, "client"); err != nil {
			return err
		}
	}
	for i, item := range lineItems {
		if item.ProductID != nil && *item.ProductID != "" {
			product, err := d.GetProduct(*item.ProductID)
			if err != nil {
				return newValidationError("line %d: product not found", i+1)
			}
			if err := requireSameOrg(organizationID, product.OrganizationID, fmt.Sprintf("line %d: product", i+1)); err != nil {
				return err
			}
		}
	}
	return nil
}

// NextOrderNumber proposes the next number for an organization, formatted
// from its configured document_number_settings format (or the
// "ORD-%03d"-equivalent default) and its persisted counter + 1 — see
// db/document_number.go. Read-only; does not consume the number. Unlike the
// other four document types, this had no server-side equivalent at all
// before — the frontend's nextOrderNumberAtom used to scan whatever orders
// were already loaded into ordersAtom, which silently proposed a stale or
// wrong number under pagination or filtering.
func (d *Database) NextOrderNumber(organizationID string) string {
	number, err := d.PreviewNextDocumentNumber(organizationID, "order")
	if err != nil {
		return ""
	}
	return number
}

func (d *Database) CreateOrder(req CreateOrderRequest) (*Order, error) {
	// F94: an empty-string optional FK id means "unset"; normalize it to
	// nil before the guard and the INSERT both see it (db/optional_id.go).
	req.ClientID = nilIfEmptyID(req.ClientID)
	normalizeOrderLineItemIDs(req.LineItems)

	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	if !validOrderStatuses[req.Status] {
		return nil, newValidationError("invalid order status %q", req.Status)
	}
	if err := d.checkOrderFKOwnership(req.OrganizationID, req.ClientID, req.LineItems); err != nil {
		return nil, err
	}
	org, err := d.GetOrganization(req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("create_order organization: %w", err)
	}
	exchangeRate, err := resolveExchangeRateForSave(
		orgCurrencyOrDefault(org), "", nil, req.Currency, req.ExchangeRate,
	)
	if err != nil {
		return nil, err
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_order begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Always advances document_number_settings' persisted counter for this
	// org+type by one, regardless of whether OrderNumber ends up storing the
	// generated value or one the client edited (see
	// db/document_number.go's doc comment). Replaces the old client-side-only
	// nextOrderNumberAtom, which scanned whatever orders happened to already
	// be loaded in the browser — fragile under pagination/filtering.
	generatedNumber, err := GenerateNextDocumentNumberTx(tx, req.OrganizationID, "order", time.UnixMilli(req.OrderDate), "")
	if err != nil {
		return nil, err
	}
	if req.OrderNumber == "" {
		req.OrderNumber = generatedNumber
	}

	_, err = tx.Exec(`
		INSERT INTO orders (id, organizationId, clientId, orderNumber, status, orderDate, deliveryDate,
		                     currency, exchangeRate, exchangeRateDate, shippingAddress, trackingNumber, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.ClientID, req.OrderNumber, req.Status,
		req.OrderDate, req.DeliveryDate, req.Currency, exchangeRate, req.ExchangeRateDate,
		req.ShippingAddress, req.TrackingNumber, req.Notes,
	)
	if err != nil {
		return nil, fmt.Errorf("create_order insert: %w", err)
	}

	if err := replaceOrderLineItemsTx(tx, req.ID, req.LineItems); err != nil {
		return nil, fmt.Errorf("create_order line_items: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_order commit: %w", err)
	}

	return d.GetOrder(req.ID)
}

func (d *Database) UpdateOrder(orderID string, updates UpdateOrderRequest) (*Order, error) {
	// F94: an empty-string optional FK id means "unset"; normalize it to
	// nil before the guard and the INSERT both see it (db/optional_id.go).
	updates.ClientID = nilIfEmptyID(updates.ClientID)
	if updates.LineItems != nil {
		normalizeOrderLineItemIDs(*updates.LineItems)
	}

	current, err := d.GetOrder(orderID)
	if err != nil {
		return nil, fmt.Errorf("update_order fetch current: %w", err)
	}
	var updateLineItems []CreateOrderLineItemRequest
	if updates.LineItems != nil {
		updateLineItems = *updates.LineItems
	}
	if err := d.checkOrderFKOwnership(current.OrganizationID, updates.ClientID, updateLineItems); err != nil {
		return nil, err
	}
	org, err := d.GetOrganization(current.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("update_order organization: %w", err)
	}
	currentCurrency := ""
	if current.Currency != nil {
		currentCurrency = *current.Currency
	}
	exchangeRate, err := resolveExchangeRateForSave(
		orgCurrencyOrDefault(org), currentCurrency, current.ExchangeRate,
		updates.Currency, updates.ExchangeRate,
	)
	if err != nil {
		return nil, err
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_order begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		UPDATE orders
		SET clientId         = ?,
		    orderNumber      = COALESCE(?, orderNumber),
		    orderDate        = COALESCE(?, orderDate),
		    deliveryDate     = ?,
		    currency         = ?,
		    exchangeRate     = ?,
		    exchangeRateDate = ?,
		    shippingAddress  = ?,
		    trackingNumber   = ?,
		    notes            = ?
		WHERE id = ?`,
		updates.ClientID,
		updates.OrderNumber, updates.OrderDate,
		updates.DeliveryDate, updates.Currency, exchangeRate, updates.ExchangeRateDate,
		updates.ShippingAddress, updates.TrackingNumber, updates.Notes,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("update_order exec: %w", err)
	}

	if updates.LineItems != nil {
		if err := replaceOrderLineItemsTx(tx, orderID, *updates.LineItems); err != nil {
			return nil, fmt.Errorf("update_order line_items: %w", err)
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

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_order_status begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// F66 (2026-08-13 fix-review): re-check the status the transition above
	// was validated against, now that this transaction actually holds the
	// connection — the same guard F48 added to every GL/stock-affecting
	// status path (UpdateInvoiceState, UpdateDeliveryStatus, ...).
	res, err := tx.Exec(
		`UPDATE orders SET status = ? WHERE id = ? AND status = ?`, status, orderID, current.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("update_order_status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("update_order_status rows_affected: %w", err)
	}
	if n == 0 {
		return nil, newValidationError("order status changed by another request — reload and try again")
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_order_status commit: %w", err)
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

	// F103: re-read the status through tx and repeat the predicate in the
	// DELETE — a concurrent PATCH .../status that moved the order to
	// shipped/delivered between the pre-tx read above and this Beginx could
	// otherwise let the delete remove an order whose deliveries/line items
	// have advanced. Same guard shape as DeleteJournalEntry/
	// DeleteProductionOrder.
	var liveStatus string
	if err := tx.Get(&liveStatus, `SELECT status FROM orders WHERE id = ?`, orderID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("delete_order status_check: %w", err)
	}
	if liveStatus == "shipped" || liveStatus == "delivered" {
		return false, newValidationError("cannot delete a %s order — cancel it instead", liveStatus)
	}

	if _, err = tx.Exec(`DELETE FROM orderLineItems WHERE orderId = ?`, orderID); err != nil {
		return false, fmt.Errorf("delete_order items: %w", err)
	}

	res, err := tx.Exec(
		`DELETE FROM orders WHERE id = ? AND status NOT IN ('shipped', 'delivered')`, orderID,
	)
	if err != nil {
		return false, fmt.Errorf("delete_order: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("delete_order commit: %w", err)
	}

	n, _ := res.RowsAffected()
	return n > 0, nil
}

// replaceOrderLineItemsTx writes an order's line items, reusing the row id
// of every line the request still carries so
// outbound_delivery_line_items.orderLineItemId survives the edit. See
// db/line_item_reconcile.go for why (F70) and for what the other,
// deliberately untouched, line-item tables do instead.
func replaceOrderLineItemsTx(exec sqlSelectExecer, orderID string, items []CreateOrderLineItemRequest) error {
	requested := make([]*string, len(items))
	for i, item := range items {
		requested[i] = item.ID
	}
	slots, obsolete, err := reconcileLineItemIDs(exec, "orderLineItems", "orderId", orderID, requested)
	if err != nil {
		return err
	}
	if err := deleteLineItemsByID(exec, "orderLineItems", "orderId", orderID, obsolete); err != nil {
		return err
	}

	for i, item := range items {
		if slots[i].Existing {
			if _, err := exec.Exec(`
				UPDATE orderLineItems
				SET productId = ?, description = ?, quantity = ?, unitPrice = ?, position = ?
				WHERE id = ? AND orderId = ?`,
				item.ProductID, item.Description, item.Quantity, roundCents(item.UnitPrice), i,
				slots[i].ID, orderID,
			); err != nil {
				return fmt.Errorf("update_order_line_item: %w", err)
			}
			continue
		}
		if _, err := exec.Exec(`
			INSERT INTO orderLineItems (id, orderId, productId, description, quantity, unitPrice, position)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			slots[i].ID, orderID, item.ProductID, item.Description, item.Quantity,
			roundCents(item.UnitPrice), i,
		); err != nil {
			return fmt.Errorf("insert_order_line_item: %w", err)
		}
	}
	return nil
}
