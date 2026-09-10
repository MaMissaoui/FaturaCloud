package db

import (
	"database/sql"
	"fmt"
	"math"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// ValidationError marks a message that's already safe to show the end user
// verbatim — a business-rule rejection (e.g. insufficient stock, invalid
// status transition), not an internal failure whose details should stay
// server-side.
type ValidationError struct{ msg string }

func (e *ValidationError) Error() string { return e.msg }

func newValidationError(format string, args ...any) error {
	return &ValidationError{msg: fmt.Sprintf(format, args...)}
}

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
	// OwnClientID is this delivery's own directly-recorded client (migration
	// 0064) — only ever meaningful (and only ever settable) when OrderID is
	// nil, since an order-backed delivery's client comes from the order
	// instead. Exposed as "ownClientId" so API consumers keep reading the
	// *effective* client — order's client if there is one, else this — via
	// ClientID below, unchanged from before this field existed.
	OwnClientID *string `db:"clientId" json:"ownClientId"`
	// Joined
	OrderNumber *string `db:"orderNumber"       json:"orderNumber"`
	ClientID    *string `db:"effectiveClientId" json:"clientId"`
	ClientName  *string `db:"clientName"        json:"clientName"`
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
	Serialized     *int     `db:"serialized"     json:"serialized"`
	SKU            *string  `db:"sku"            json:"sku"`
}

type CreateDeliveryLineItemRequest struct {
	OrderLineItemID *string `json:"orderLineItemId"`
	ProductID       *string `json:"productId"`
	Description     string  `json:"description"`
	Quantity        float64 `json:"quantity"`
	Unit            *string `json:"unit"`
}

type CreateDeliveryRequest struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	OrderID        *string `json:"orderId"`
	// ClientID is only meaningful for a standalone delivery (OrderID nil) —
	// an order-backed delivery's effective client always comes from the
	// order instead (see outboundDeliverySelect). Stored regardless of
	// OrderID for simplicity; it just goes unused/hidden by the COALESCE
	// whenever an order is also linked.
	ClientID        *string                         `json:"clientId"`
	DeliveryNumber  string                          `json:"deliveryNumber"`
	DeliveryDate    int64                           `json:"deliveryDate"`
	ShippingAddress *string                         `json:"shippingAddress"`
	TrackingNumber  *string                         `json:"trackingNumber"`
	Notes           *string                         `json:"notes"`
	LineItems       []CreateDeliveryLineItemRequest `json:"lineItems"`
}

type UpdateDeliveryRequest struct {
	OrderID         *string                          `json:"orderId"`
	ClientID        *string                          `json:"clientId"`
	DeliveryNumber  *string                          `json:"deliveryNumber"`
	DeliveryDate    *int64                           `json:"deliveryDate"`
	ShippingAddress *string                          `json:"shippingAddress"`
	TrackingNumber  *string                          `json:"trackingNumber"`
	Notes           *string                          `json:"notes"`
	LineItems       *[]CreateDeliveryLineItemRequest `json:"lineItems"`
}

// outboundDeliverySelect resolves the *effective* client — the linked
// order's client when there is one, else the delivery's own directly-set
// clientId (migration 0064) — as effectiveClientId, joining clients once
// against whichever of those two ids applies rather than twice (order-client
// vs own-client) and coalescing names, since exactly one of them is ever
// non-null for a given row.
const outboundDeliverySelect = `
	SELECT od.*,
	       o.orderNumber AS orderNumber,
	       COALESCE(o.clientId, od.clientId) AS effectiveClientId,
	       c.name AS clientName
	FROM outbound_deliveries od
	LEFT JOIN orders o ON od.orderId = o.id
	LEFT JOIN clients c ON c.id = COALESCE(o.clientId, od.clientId)`

func (d *Database) GetDeliveries(organizationID string) ([]OutboundDelivery, error) {
	rows := []OutboundDelivery{}
	err := d.DB.Select(&rows, outboundDeliverySelect+`
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
	err := d.DB.Get(&row, outboundDeliverySelect+`
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
		       p.stockQuantity AS availableStock,
		       p.serialized AS serialized,
		       p.sku AS sku
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

// checkDeliveryHeaderFKOwnership validates that orderId/clientId (if set)
// belong to the SAME organization as the delivery (issue #189) — a line
// item's orderLineItemId/productId are checked separately, inside
// replaceDeliveryLineItemsTx, since resolving a line's product from its
// order line requires the same query either way.
func (d *Database) checkDeliveryHeaderFKOwnership(organizationID string, orderID, clientID *string) error {
	if orderID != nil && *orderID != "" {
		order, err := d.GetOrder(*orderID)
		if err != nil {
			return newValidationError("order not found")
		}
		if err := requireSameOrg(organizationID, order.OrganizationID, "order"); err != nil {
			return err
		}
	}
	if clientID != nil && *clientID != "" {
		client, err := d.GetClient(*clientID)
		if err != nil {
			return newValidationError("client not found")
		}
		if err := requireSameOrg(organizationID, client.OrganizationID, "client"); err != nil {
			return err
		}
	}
	return nil
}

func (d *Database) CreateDelivery(req CreateDeliveryRequest) (*OutboundDelivery, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	if err := d.checkDeliveryHeaderFKOwnership(req.OrganizationID, req.OrderID, req.ClientID); err != nil {
		return nil, err
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_delivery begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		INSERT INTO outbound_deliveries
		  (id, organizationId, orderId, clientId, deliveryNumber, deliveryDate, shippingAddress, trackingNumber, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.OrderID, req.ClientID, req.DeliveryNumber,
		req.DeliveryDate, req.ShippingAddress, req.TrackingNumber, req.Notes,
	)
	if err != nil {
		return nil, fmt.Errorf("create_delivery: %w", err)
	}
	if err := replaceDeliveryLineItemsTx(tx, req.OrganizationID, req.ID, req.LineItems); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_delivery commit: %w", err)
	}
	return d.GetDelivery(req.ID)
}

func (d *Database) UpdateDelivery(id string, req UpdateDeliveryRequest) (*OutboundDelivery, error) {
	current, err := d.GetDelivery(id)
	if err != nil {
		return nil, fmt.Errorf("update_delivery lookup: %w", err)
	}
	if req.LineItems != nil && (current.Status == "shipped" || current.Status == "delivered") {
		return nil, newValidationError("cannot edit line items of a %s delivery", current.Status)
	}
	if err := d.checkDeliveryHeaderFKOwnership(current.OrganizationID, req.OrderID, req.ClientID); err != nil {
		return nil, err
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_delivery begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		UPDATE outbound_deliveries SET
		  orderId         = COALESCE(?, orderId),
		  clientId        = COALESCE(?, clientId),
		  deliveryNumber  = COALESCE(?, deliveryNumber),
		  deliveryDate    = COALESCE(?, deliveryDate),
		  shippingAddress = COALESCE(?, shippingAddress),
		  trackingNumber  = COALESCE(?, trackingNumber),
		  notes           = COALESCE(?, notes)
		WHERE id = ?`,
		req.OrderID, req.ClientID, req.DeliveryNumber, req.DeliveryDate,
		req.ShippingAddress, req.TrackingNumber, req.Notes,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("update_delivery: %w", err)
	}
	if req.LineItems != nil {
		if err := replaceDeliveryLineItemsTx(tx, current.OrganizationID, id, *req.LineItems); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_delivery commit: %w", err)
	}
	return d.GetDelivery(id)
}

// deliveryStockLine is a delivery line item resolved to its stock-enabled product.
type deliveryStockLine struct {
	LineItemID     string  `db:"lineItemId"`
	ProductID      string  `db:"productId"`
	ProductName    string  `db:"productName"`
	Quantity       float64 `db:"quantity"`
	AvailableStock float64 `db:"availableStock"`
	Serialized     int     `db:"serialized"`
}

// getShippableStockLines returns the delivery's line items that are linked to a
// stock-enabled product (whether picked directly or copied from an order line
// item) — the only lines that affect inventory. Takes a generic execer (not
// *sqlx.Tx) so UpdateDeliveryStatus can read it once via d.DB before its
// transaction opens — see that function's comment on why no d.DB read may
// happen once Beginx has run.
func getShippableStockLines(exec sqlSelectExecer, deliveryID string) ([]deliveryStockLine, error) {
	lines := []deliveryStockLine{}
	err := exec.Select(&lines, `
		SELECT dli.id AS lineItemId,
		       p.id AS productId,
		       p.name AS productName,
		       dli.quantity AS quantity,
		       p.stockQuantity AS availableStock,
		       p.serialized AS serialized
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

// deliveryStatusTransitions enumerates the only legal delivery status moves;
// "delivered" and "cancelled" are terminal (absent as keys, so any move out
// of them is rejected). Mirrors src/routes/deliveries/details.tsx's
// STATUS_TRANSITIONS, enforced here too since that's client-side only.
var deliveryStatusTransitions = map[string]map[string]bool{
	"draft":   {"shipped": true, "cancelled": true},
	"shipped": {"delivered": true, "cancelled": true},
}

// UpdateDeliveryStatus updates a delivery's status, reducing inventory when a
// draft delivery is marked shipped (rejecting the transition if any stock-enabled
// product doesn't have enough available stock) and restoring inventory when an
// already-shipped delivery is cancelled. Any transition not in
// deliveryStatusTransitions (including out of a terminal state) is rejected;
// setting a delivery to its current status is a no-op.
//
// serialNumbers is keyed by line-item id and is required (exactly matching
// each line's quantity, each already in stock) for any line whose product is
// serialized — mandatory so the registry never falls behind actual stock
// movements. It's ignored for non-serialized lines and unused on any
// transition other than draft->shipped.
func (d *Database) UpdateDeliveryStatus(id, status string, serialNumbers map[string][]string) (*OutboundDelivery, error) {
	current, err := d.GetDelivery(id)
	if err != nil {
		return nil, fmt.Errorf("update_delivery_status lookup: %w", err)
	}
	if status != current.Status && !deliveryStatusTransitions[current.Status][status] {
		return nil, newValidationError("cannot transition delivery from %q to %q", current.Status, status)
	}

	// Phase 7: every d.DB read either transition needs (the delivery's own
	// line items, serial lookups, its COGS posting/reversal lines, the
	// organization's GL defaults) happens here, before the transaction
	// opens — db.SetMaxOpenConns(1) means a second connection request
	// while a tx holds the only one blocks forever rather than erroring,
	// so no d.DB read may happen between Beginx and Commit below (see
	// UpdateInvoiceState's identical constraint). This is also why a
	// shipment with no cost basis never touches stock at all: the 409 from
	// buildDeliveryCOGSGLLines happens before Beginx, so there's no
	// transaction to roll back — nothing was ever attempted.
	var lines []deliveryStockLine
	var resolvedSerials map[string]map[string]string
	var cogsLines []CreateJournalLineRequest
	var cogsJournal *Journal
	var existingCOGSEntry *JournalEntry

	switch {
	case current.Status == "draft" && status == "shipped":
		lines, err = getShippableStockLines(d.DB, id)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			if line.Serialized == 1 {
				continue // validated per-serial below, a superset of this aggregate check
			}
			if line.Quantity > line.AvailableStock {
				return nil, newValidationError(
					"insufficient stock for %q: available %.2f, requested %.2f",
					line.ProductName, line.AvailableStock, line.Quantity,
				)
			}
		}

		// Validate every serialized line before writing anything: whole-number
		// quantity, exactly that many (deduped) serials supplied, no serial
		// reused across lines of the same product in this one request, and —
		// stricter than receive — every serial must already be registered
		// *and* currently in stock (an unregistered serial obviously isn't in
		// this org's stock, so it fails the same way).
		perProductSerials := map[string][]string{}
		for _, line := range lines {
			if line.Serialized != 1 {
				continue
			}
			if line.Quantity != math.Trunc(line.Quantity) {
				return nil, newValidationError(
					"%q is serialized — quantity must be a whole number, got %.2f",
					line.ProductName, line.Quantity,
				)
			}
			given := dedupeStrings(serialNumbers[line.LineItemID])
			if len(given) != int(line.Quantity) {
				return nil, newValidationError(
					"%q requires exactly %d serial number(s), got %d",
					line.ProductName, int(line.Quantity), len(given),
				)
			}
			perProductSerials[line.ProductID] = append(perProductSerials[line.ProductID], given...)
		}
		resolvedSerials = map[string]map[string]string{} // productId -> serial -> serialNumberId
		for productID, serials := range perProductSerials {
			if len(dedupeStrings(serials)) != len(serials) {
				return nil, newValidationError("duplicate serial number(s) supplied for the same product in this delivery")
			}
			lookups, err := lookupSerialNumbersTx(d.DB, productID, serials)
			if err != nil {
				return nil, err
			}
			ids := make(map[string]string, len(serials))
			for _, s := range serials {
				lu, ok := lookups[s]
				if !ok || !lu.InStock {
					return nil, newValidationError("serial %q is not currently in stock", s)
				}
				ids[s] = lu.ID
			}
			resolvedSerials[productID] = ids
		}

		cogsLines, cogsJournal, err = buildDeliveryCOGSGLLines(d, current, lines, resolvedSerials)
		if err != nil {
			return nil, err
		}

	case current.Status == "shipped" && status == "cancelled":
		lines, err = getShippableStockLines(d.DB, id)
		if err != nil {
			return nil, err
		}
		existingCOGSEntry, err = d.FindPostedEntryForSourceDocument("outbound_delivery", current.ID)
		if err != nil {
			return nil, err
		}
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_delivery_status begin: %w", err)
	}
	defer tx.Rollback()

	// F48: re-check the status this whole function's decisions were made
	// against (which lines to move, whether cogsLines/existingCOGSEntry
	// apply) now that this transaction actually holds the connection — a
	// concurrent request against the same delivery could have already
	// shipped or cancelled it in the time between the pre-tx read above and
	// Beginx() acquiring the connection.
	var liveStatus string
	if err := tx.Get(&liveStatus, `SELECT status FROM outbound_deliveries WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("update_delivery_status status_check: %w", err)
	}
	if liveStatus != current.Status {
		return nil, newValidationError(
			"delivery status changed to %q by another request — reload and try again", liveStatus,
		)
	}

	touchedProducts := []string{}

	switch {
	case current.Status == "draft" && status == "shipped":
		for _, line := range lines {
			if line.Serialized == 1 {
				serials := dedupeStrings(serialNumbers[line.LineItemID])
				for _, s := range serials {
					serialID := resolvedSerials[line.ProductID][s]
					movementID, _ := gonanoid.New()
					if err := insertStockMovementRowTx(tx, CreateStockMovementRequest{
						ID:               movementID,
						OrganizationID:   current.OrganizationID,
						ProductID:        line.ProductID,
						Type:             "out",
						Quantity:         -1,
						Note:             ptrStr("Delivery " + current.DeliveryNumber),
						Reference:        &current.DeliveryNumber,
						SerialNumberID:   &serialID,
						SourceDocumentID: &current.ID,
					}); err != nil {
						return nil, fmt.Errorf("update_delivery_status reduce_stock: %w", err)
					}
				}
				if err := recomputeStockQuantityTx(tx, line.ProductID); err != nil {
					return nil, err
				}
				touchedProducts = append(touchedProducts, line.ProductID)
				continue
			}

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
			touchedProducts = append(touchedProducts, line.ProductID)
		}

		// Phase 7: post COGS for whatever this shipment actually costs
		// (built before the transaction opened, above) — the entry balances
		// by construction, so if buildDeliveryCOGSGLLines had found a line
		// with no cost basis, it already returned a 409 before any stock
		// movement here was ever attempted.
		if cogsLines != nil {
			if _, err := postAutoEntryTx(
				tx, current.OrganizationID, cogsJournal.ID, "outbound_delivery", current.ID,
				current.DeliveryDate, current.DeliveryNumber, "Delivery "+current.DeliveryNumber, cogsLines,
			); err != nil {
				return nil, err
			}
		}

	case current.Status == "shipped" && status == "cancelled":
		// Serialized products dedupe by productId here (not per line) since
		// reversal looks up everything this delivery posted for that product
		// across all its lines combined, not line-by-line.
		serializedProductNames := map[string]string{}
		for _, line := range lines {
			if line.Serialized == 1 {
				serializedProductNames[line.ProductID] = line.ProductName
			}
		}

		// For each serialized product, resolve exactly the serial units this
		// delivery shipped — via sourceDocumentId, never `reference` (free
		// text a manual movement could coincidentally reuse) — and confirm
		// none has already come back into stock through some other movement
		// before restoring any of them.
		postedByProduct := map[string][]string{}
		for productID, productName := range serializedProductNames {
			var posted []struct {
				SerialNumberID string `db:"serialNumberId"`
			}
			if err := tx.Select(&posted, `
				SELECT serialNumberId FROM stockMovements
				WHERE productId = ? AND sourceDocumentId = ? AND type = 'out' AND serialNumberId IS NOT NULL`,
				productID, current.ID,
			); err != nil {
				return nil, fmt.Errorf("update_delivery_status lookup_serials: %w", err)
			}
			ids := make([]string, len(posted))
			for i, p := range posted {
				ids[i] = p.SerialNumberID
			}
			inStock, err := serialInStockByIDTx(tx, ids)
			if err != nil {
				return nil, err
			}
			for _, sid := range ids {
				if inStock[sid] {
					return nil, newValidationError(
						"cannot cancel: %q has a unit shipped by this delivery that's already back in stock through another movement",
						productName,
					)
				}
			}
			postedByProduct[productID] = ids
		}

		for _, line := range lines {
			if line.Serialized == 1 {
				continue // restored once per product below, not per line
			}
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
			touchedProducts = append(touchedProducts, line.ProductID)
		}
		for productID, ids := range postedByProduct {
			for _, sid := range ids {
				serialID := sid
				movementID, _ := gonanoid.New()
				if err := insertStockMovementRowTx(tx, CreateStockMovementRequest{
					ID:               movementID,
					OrganizationID:   current.OrganizationID,
					ProductID:        productID,
					Type:             "in",
					Quantity:         1,
					Note:             ptrStr("Delivery " + current.DeliveryNumber + " cancelled"),
					Reference:        &current.DeliveryNumber,
					SerialNumberID:   &serialID,
					SourceDocumentID: &current.ID,
				}); err != nil {
					return nil, fmt.Errorf("update_delivery_status restore_stock: %w", err)
				}
			}
			if err := recomputeStockQuantityTx(tx, productID); err != nil {
				return nil, err
			}
			touchedProducts = append(touchedProducts, productID)
		}

		// Phase 7: reverse the shipment's COGS entry, if it posted one — a
		// delivery of only free items never did (buildDeliveryCOGSGLLines
		// returns nil), and that's a no-op here, not an error.
		if existingCOGSEntry != nil {
			if _, err := reverseEntryTx(tx, existingCOGSEntry, "delivery cancelled", current.DeliveryDate); err != nil {
				return nil, err
			}
		}
	}

	// Outflows never move the weighted average, but recompute at every movement
	// site so nobody has to re-derive why one path would be exempt.
	for _, productID := range touchedProducts {
		if err := recomputeAverageCostTx(tx, productID); err != nil {
			return nil, fmt.Errorf("update_delivery_status recompute_cost: %w", err)
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
		return false, newValidationError("cannot delete a %s delivery — cancel it instead", current.Status)
	}

	res, err := d.DB.Exec(`DELETE FROM outbound_deliveries WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete_delivery: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// sqlGetExecer is satisfied by both *sqlx.DB and *sqlx.Tx, letting
// replaceDeliveryLineItemsTx run as part of a caller's transaction (CreateDelivery,
// UpdateDelivery) while still resolving each line's product via a plain SELECT.
type sqlGetExecer interface {
	sqlExecer
	Get(dest any, query string, args ...any) error
}

// replaceDeliveryLineItemsTx also enforces issue #189's ownership rule for
// this table's two cross-org-referenceable fields: orderLineItemId (via its
// parent order) and productId, checked with the same tx-capable exec used
// for every other read/write here rather than a separate d.DB call — this
// runs inside CreateDelivery/UpdateDelivery's already-open transaction, and
// SetMaxOpenConns(1) means a second connection from d.DB would deadlock.
func replaceDeliveryLineItemsTx(exec sqlGetExecer, organizationID, deliveryID string, items []CreateDeliveryLineItemRequest) error {
	_, err := exec.Exec(`DELETE FROM outbound_delivery_line_items WHERE deliveryId = ?`, deliveryID)
	if err != nil {
		return fmt.Errorf("delete_delivery_line_items: %w", err)
	}
	for i, item := range items {
		id, _ := gonanoid.New()
		productID := item.ProductID
		if item.OrderLineItemID != nil {
			var resolved struct {
				ProductID      sql.NullString `db:"productId"`
				OrganizationID string         `db:"organizationId"`
			}
			if err := exec.Get(&resolved, `
				SELECT ol.productId AS productId, o.organizationId AS organizationId
				FROM orderLineItems ol
				JOIN orders o ON o.id = ol.orderId
				WHERE ol.id = ?`, *item.OrderLineItemID,
			); err != nil {
				return newValidationError("line %d: order line item not found", i+1)
			}
			if err := requireSameOrg(organizationID, resolved.OrganizationID, fmt.Sprintf("line %d: order line item", i+1)); err != nil {
				return err
			}
			if productID == nil && resolved.ProductID.Valid {
				productID = &resolved.ProductID.String
			}
		}
		if productID != nil && *productID != "" {
			var productOrgID string
			if err := exec.Get(&productOrgID, `SELECT organizationId FROM products WHERE id = ?`, *productID); err != nil {
				return newValidationError("line %d: product not found", i+1)
			}
			if err := requireSameOrg(organizationID, productOrgID, fmt.Sprintf("line %d: product", i+1)); err != nil {
				return err
			}
		}
		_, err := exec.Exec(`
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

// NextDeliveryNumber proposes the next "DEL-%04d" number for an organization,
// continuing from the highest number in use rather than COUNT(*)+1 — the
// latter reissues an already-used number as soon as any delivery is deleted,
// since deliveryNumber has no UNIQUE constraint to catch the collision.
func (d *Database) NextDeliveryNumber(organizationID string) string {
	var maxNumber sql.NullInt64
	_ = d.DB.Get(&maxNumber, `
		SELECT MAX(CAST(SUBSTR(deliveryNumber, 5) AS INTEGER))
		FROM outbound_deliveries
		WHERE organizationId = ? AND deliveryNumber LIKE 'DEL-%'`,
		organizationID,
	)
	return fmt.Sprintf("DEL-%04d", maxNumber.Int64+1)
}
