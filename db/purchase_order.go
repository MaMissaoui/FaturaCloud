package db

import (
	"database/sql"
	"errors"
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

type PurchaseOrder struct {
	ID             string  `db:"id"              json:"id"`
	OrganizationID string  `db:"organizationId"  json:"organizationId"`
	VendorID       *string `db:"vendorId"        json:"vendorId"`
	OrderNumber    string  `db:"orderNumber"     json:"orderNumber"`
	Status         string  `db:"status"          json:"status"`
	OrderDate      int64   `db:"orderDate"       json:"orderDate"`
	ExpectedDate   *int64  `db:"expectedDate"    json:"expectedDate"`
	Currency       *string `db:"currency"        json:"currency"`
	// See db/exchange_rate.go for the rate direction convention.
	ExchangeRate     *string `db:"exchangeRate"     json:"exchangeRate"`
	ExchangeRateDate *int64  `db:"exchangeRateDate" json:"exchangeRateDate"`
	DeliveryAddress  *string `db:"deliveryAddress" json:"deliveryAddress"`
	Notes            *string `db:"notes"           json:"notes"`
	// F114: the shipment this order's goods travel in, if any — drives
	// landed cost allocation at receiving time (db/gl_posting.go's
	// applyLandedCost). No ON DELETE clause, same precedent as vendorId
	// below; guarded app-side by DeleteImport.
	ImportID     *string `db:"importId"        json:"importId"`
	VendorName   *string `db:"vendorName"      json:"vendorName"`
	ImportNumber *string `db:"importNumber"    json:"importNumber"`
	CreatedAt    int64   `db:"createdAt"       json:"createdAt"`
}

type PurchaseOrderLineItem struct {
	ID              string  `db:"id"              json:"id"`
	PurchaseOrderID string  `db:"purchaseOrderId" json:"purchaseOrderId"`
	ProductID       *string `db:"productId"       json:"productId"`
	Description     string  `db:"description"     json:"description"`
	Quantity        float64 `db:"quantity"        json:"quantity"`
	UnitPrice       int64   `db:"unitPrice"       json:"unitPrice"`
	Unit            *string `db:"unit"            json:"unit"`
	TaxRate         *string `db:"taxRate"         json:"taxRate"`
	Position        int     `db:"position"        json:"position"`
	// Joined from products via productId; nil on a free-text line or an
	// unset SKU — same convention as OutboundDeliveryLineItem.SKU.
	SKU *string `db:"sku" json:"sku"`
}

type CreatePurchaseOrderLineItemRequest struct {
	ProductID   *string `json:"productId"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice"` // cents sent from frontend
	Unit        *string `json:"unit"`
	TaxRate     *string `json:"taxRate"`
}

type CreatePurchaseOrderRequest struct {
	ID               string                               `json:"id"`
	OrganizationID   string                               `json:"organizationId"`
	VendorID         *string                              `json:"vendorId"`
	OrderNumber      string                               `json:"orderNumber"`
	Status           string                               `json:"status"`
	OrderDate        int64                                `json:"orderDate"`
	ExpectedDate     *int64                               `json:"expectedDate"`
	Currency         *string                              `json:"currency"`
	ExchangeRate     *float64                             `json:"exchangeRate"`
	ExchangeRateDate *int64                               `json:"exchangeRateDate"`
	DeliveryAddress  *string                              `json:"deliveryAddress"`
	Notes            *string                              `json:"notes"`
	ImportID         *string                              `json:"importId"`
	LineItems        []CreatePurchaseOrderLineItemRequest `json:"lineItems"`
}

// UpdatePurchaseOrderRequest deliberately has no Status field — status changes
// go through PATCH /api/purchase-orders/{id}/status only, so a PUT can't skip
// the transition matrix.
type UpdatePurchaseOrderRequest struct {
	VendorID         *string                               `json:"vendorId"`
	OrderNumber      *string                               `json:"orderNumber"`
	OrderDate        *int64                                `json:"orderDate"`
	ExpectedDate     *int64                                `json:"expectedDate"`
	Currency         *string                               `json:"currency"`
	ExchangeRate     *float64                              `json:"exchangeRate"`
	ExchangeRateDate *int64                                `json:"exchangeRateDate"`
	DeliveryAddress  *string                               `json:"deliveryAddress"`
	Notes            *string                               `json:"notes"`
	ImportID         *string                               `json:"importId"`
	LineItems        *[]CreatePurchaseOrderLineItemRequest `json:"lineItems"`
}

// validPurchaseOrderStatuses are the only values purchase_orders.status may
// take; the CHECK constraint in migration 0030 enforces the same set at the
// database level.
var validPurchaseOrderStatuses = map[string]bool{
	"draft": true, "confirmed": true, "received": true, "cancelled": true,
}

// purchaseOrderStatusTransitions enumerates the only legal moves; "received"
// is terminal (absent as a key, so any move out of it is rejected).
// "cancelled" is NOT terminal — it can fall back to "confirmed" or
// "received" (a correction for a cancellation made in error, or one that's
// no longer wanted), which is safe to allow unconditionally here because
// purchase order status changes have zero side effects (no stock, no GL
// posting — contrast db/delivery.go's UpdateDeliveryStatus and
// db/inbound_delivery.go's UpdateInboundDeliveryStatus, where "cancelled"
// stays deliberately terminal since reversing it would mean re-deriving
// stock movements and GL entries, not just flipping a status column).
// Mirrors purchaseOrderStatusTransitionMatrix in src/types/purchase-order.ts,
// enforced here too since that's client-side only.
//
// cancelled -> received deliberately skips confirmed — this means a PO can
// now reach "received" without ever having been "confirmed", a path the
// forward flow itself doesn't allow. That's intentional, not an oversight:
// nothing downstream gates on having passed through "confirmed" (only
// totalCommittedPOValueForImport in db/import.go excludes "cancelled", by
// current status, not history), so there's nothing to protect by forcing a
// detour through it here.
//
// Status is never advanced automatically from received quantities — sales
// orders don't either, and per-line fulfilment is reported separately.
var purchaseOrderStatusTransitions = map[string]map[string]bool{
	"draft":     {"confirmed": true, "cancelled": true},
	"confirmed": {"received": true, "cancelled": true},
	"cancelled": {"confirmed": true, "received": true},
}

func (d *Database) GetPurchaseOrders(organizationID string) ([]PurchaseOrder, error) {
	orders := []PurchaseOrder{}
	err := d.DB.Select(&orders, `
		SELECT purchase_orders.*, vendors.name AS vendorName, imports.importNumber AS importNumber
		FROM purchase_orders
		LEFT JOIN vendors ON purchase_orders.vendorId = vendors.id
		LEFT JOIN imports ON purchase_orders.importId = imports.id
		WHERE purchase_orders.organizationId = ?
		ORDER BY purchase_orders.orderDate DESC, purchase_orders.createdAt DESC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_purchase_orders: %w", err)
	}
	return orders, nil
}

func (d *Database) GetPurchaseOrder(orderID string) (*PurchaseOrder, error) {
	var order PurchaseOrder
	err := d.DB.Get(&order, `
		SELECT purchase_orders.*, vendors.name AS vendorName, imports.importNumber AS importNumber
		FROM purchase_orders
		LEFT JOIN vendors ON purchase_orders.vendorId = vendors.id
		LEFT JOIN imports ON purchase_orders.importId = imports.id
		WHERE purchase_orders.id = ?
		LIMIT 1`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_purchase_order: %w", err)
	}
	return &order, nil
}

func (d *Database) GetPurchaseOrderLineItems(orderID string) ([]PurchaseOrderLineItem, error) {
	items := []PurchaseOrderLineItem{}
	err := d.DB.Select(&items,
		`SELECT poli.*, p.sku AS sku
		 FROM purchase_order_line_items poli
		 LEFT JOIN products p ON poli.productId = p.id
		 WHERE poli.purchaseOrderId = ? ORDER BY poli.position ASC`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_purchase_order_line_items: %w", err)
	}
	return items, nil
}

// NextPurchaseOrderNumber proposes the next "PO-%04d" number for an
// organization, continuing from the highest number in use rather than
// COUNT(*)+1 — the latter reissues an already-used number as soon as any order
// is deleted, since orderNumber has no UNIQUE constraint to catch the
// collision.
//
// SUBSTR starts at 4 because the prefix "PO-" is three characters; the
// equivalent for deliveries uses 5 for "DEL-".
func (d *Database) NextPurchaseOrderNumber(organizationID string) string {
	var maxNumber sql.NullInt64
	_ = d.DB.Get(&maxNumber, `
		SELECT MAX(CAST(SUBSTR(orderNumber, 4) AS INTEGER))
		FROM purchase_orders
		WHERE organizationId = ? AND orderNumber LIKE 'PO-%'`,
		organizationID,
	)
	return fmt.Sprintf("PO-%04d", maxNumber.Int64+1)
}

// checkPurchaseOrderFKOwnership validates that vendorId/importId (if set)
// and each line item's productId/taxRate belong to the SAME organization as
// the purchase order (issue #189). A nil/empty field means "not part of
// this request," not a mismatch.
func (d *Database) checkPurchaseOrderFKOwnership(organizationID string, vendorID, importID *string, lineItems []CreatePurchaseOrderLineItemRequest) error {
	if vendorID != nil && *vendorID != "" {
		vendor, err := d.GetVendor(*vendorID)
		if err != nil {
			return newValidationError("vendor not found")
		}
		if err := requireSameOrg(organizationID, vendor.OrganizationID, "vendor"); err != nil {
			return err
		}
	}
	if importID != nil && *importID != "" {
		imp, err := d.GetImport(*importID)
		if err != nil {
			return newValidationError("import not found")
		}
		if err := requireSameOrg(organizationID, imp.OrganizationID, "import"); err != nil {
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
		if item.TaxRate != nil && *item.TaxRate != "" {
			taxRate, err := d.GetTaxRate(*item.TaxRate)
			if err != nil {
				return newValidationError("line %d: tax rate not found", i+1)
			}
			if err := requireSameOrg(organizationID, taxRate.OrganizationID, fmt.Sprintf("line %d: tax rate", i+1)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Database) CreatePurchaseOrder(req CreatePurchaseOrderRequest) (*PurchaseOrder, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	if !validPurchaseOrderStatuses[req.Status] {
		return nil, newValidationError("invalid purchase order status %q", req.Status)
	}
	if err := d.checkPurchaseOrderFKOwnership(req.OrganizationID, req.VendorID, req.ImportID, req.LineItems); err != nil {
		return nil, err
	}
	org, err := d.GetOrganization(req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("create_purchase_order organization: %w", err)
	}
	exchangeRate, err := resolveExchangeRateForSave(
		orgCurrencyOrDefault(org), "", nil, req.Currency, req.ExchangeRate,
	)
	if err != nil {
		return nil, err
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_purchase_order begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		INSERT INTO purchase_orders (id, organizationId, vendorId, orderNumber, status, orderDate,
		                             expectedDate, currency, exchangeRate, exchangeRateDate, deliveryAddress, notes, importId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.VendorID, req.OrderNumber, req.Status,
		req.OrderDate, req.ExpectedDate, req.Currency, exchangeRate, req.ExchangeRateDate,
		req.DeliveryAddress, req.Notes, req.ImportID,
	)
	if err != nil {
		return nil, fmt.Errorf("create_purchase_order insert: %w", err)
	}

	if err := replacePurchaseOrderLineItemsTx(tx, req.ID, req.LineItems); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_purchase_order commit: %w", err)
	}

	return d.GetPurchaseOrder(req.ID)
}

func (d *Database) UpdatePurchaseOrder(orderID string, updates UpdatePurchaseOrderRequest) (*PurchaseOrder, error) {
	current, err := d.GetPurchaseOrder(orderID)
	if err != nil {
		return nil, fmt.Errorf("update_purchase_order fetch current: %w", err)
	}
	org, err := d.GetOrganization(current.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("update_purchase_order organization: %w", err)
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
	var updateLineItems []CreatePurchaseOrderLineItemRequest
	if updates.LineItems != nil {
		updateLineItems = *updates.LineItems
	}
	if err := d.checkPurchaseOrderFKOwnership(current.OrganizationID, updates.VendorID, updates.ImportID, updateLineItems); err != nil {
		return nil, err
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_purchase_order begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Required columns use COALESCE so an omitted field keeps its value;
	// genuinely optional ones are set unconditionally so they can be cleared.
	//
	// vendorId is COALESCE'd even though the column is nullable: the form
	// requires a vendor, and letting a partial PUT null it would orphan the
	// order from its vendor — the same outcome the deliberate absence of
	// ON DELETE SET NULL on that foreign key exists to prevent.
	_, err = tx.Exec(`
		UPDATE purchase_orders
		SET vendorId         = COALESCE(?, vendorId),
		    orderNumber      = COALESCE(?, orderNumber),
		    orderDate        = COALESCE(?, orderDate),
		    expectedDate     = ?,
		    currency         = ?,
		    exchangeRate     = ?,
		    exchangeRateDate = ?,
		    deliveryAddress  = ?,
		    notes            = ?,
		    importId         = ?
		WHERE id = ?`,
		updates.VendorID,
		updates.OrderNumber, updates.OrderDate,
		updates.ExpectedDate, updates.Currency, exchangeRate, updates.ExchangeRateDate,
		updates.DeliveryAddress, updates.Notes, updates.ImportID,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("update_purchase_order exec: %w", err)
	}

	if updates.LineItems != nil {
		if err := replacePurchaseOrderLineItemsTx(tx, orderID, *updates.LineItems); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_purchase_order commit: %w", err)
	}

	return d.GetPurchaseOrder(orderID)
}

// replacePurchaseOrderLineItemsTx clears and reinserts a purchase order's line
// items, renumbering position from the slice order.
func replacePurchaseOrderLineItemsTx(exec sqlExecer, orderID string, items []CreatePurchaseOrderLineItemRequest) error {
	if _, err := exec.Exec(
		`DELETE FROM purchase_order_line_items WHERE purchaseOrderId = ?`, orderID,
	); err != nil {
		return fmt.Errorf("delete_purchase_order_line_items: %w", err)
	}

	for i, item := range items {
		itemID, _ := gonanoid.New()
		_, err := exec.Exec(`
			INSERT INTO purchase_order_line_items
			  (id, purchaseOrderId, productId, description, quantity, unitPrice, unit, taxRate, position)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			itemID, orderID, item.ProductID, item.Description, item.Quantity,
			roundCents(item.UnitPrice), item.Unit, item.TaxRate, i,
		)
		if err != nil {
			return fmt.Errorf("insert_purchase_order_line_item: %w", err)
		}
	}
	return nil
}

// UpdatePurchaseOrderStatus updates a purchase order's status. Any transition
// not in purchaseOrderStatusTransitions (including out of a terminal
// "received"/"cancelled" state) is rejected; setting an order to its current
// status is a no-op.
func (d *Database) UpdatePurchaseOrderStatus(orderID string, status string) (*PurchaseOrder, error) {
	current, err := d.GetPurchaseOrder(orderID)
	if err != nil {
		return nil, fmt.Errorf("update_purchase_order_status lookup: %w", err)
	}
	if !validPurchaseOrderStatuses[status] {
		return nil, newValidationError("invalid purchase order status %q", status)
	}
	if status != current.Status && !purchaseOrderStatusTransitions[current.Status][status] {
		return nil, newValidationError("cannot transition purchase order from %q to %q", current.Status, status)
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_purchase_order_status begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// F66 (2026-08-13 fix-review): re-check the status the transition above
	// was validated against, now that this transaction actually holds the
	// connection — the same guard F48 added to every GL/stock-affecting
	// status path (UpdateInvoiceState, UpdateDeliveryStatus, ...).
	res, err := tx.Exec(
		`UPDATE purchase_orders SET status = ? WHERE id = ? AND status = ?`, status, orderID, current.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("update_purchase_order_status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("update_purchase_order_status rows_affected: %w", err)
	}
	if n == 0 {
		return nil, newValidationError("purchase order status changed by another request — reload and try again")
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_purchase_order_status commit: %w", err)
	}
	return d.GetPurchaseOrder(orderID)
}

func (d *Database) DeletePurchaseOrder(orderID string) (bool, error) {
	current, err := d.GetPurchaseOrder(orderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if current.Status == "received" {
		return false, newValidationError("cannot delete a received purchase order — cancel it instead")
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return false, fmt.Errorf("delete_purchase_order begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err = tx.Exec(
		`DELETE FROM purchase_order_line_items WHERE purchaseOrderId = ?`, orderID,
	); err != nil {
		return false, fmt.Errorf("delete_purchase_order items: %w", err)
	}

	res, err := tx.Exec(`DELETE FROM purchase_orders WHERE id = ?`, orderID)
	if err != nil {
		return false, fmt.Errorf("delete_purchase_order: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("delete_purchase_order commit: %w", err)
	}

	n, _ := res.RowsAffected()
	return n > 0, nil
}
