package db

import (
	"database/sql"
	"fmt"
	"math"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

type InboundDelivery struct {
	ID                 string  `db:"id"                 json:"id"`
	OrganizationID     string  `db:"organizationId"     json:"organizationId"`
	PurchaseOrderID    *string `db:"purchaseOrderId"    json:"purchaseOrderId"`
	VendorID           *string `db:"vendorId"           json:"vendorId"`
	DeliveryNumber     string  `db:"deliveryNumber"     json:"deliveryNumber"`
	DeliveryDate       int64   `db:"deliveryDate"       json:"deliveryDate"`
	VendorDeliveryNote *string `db:"vendorDeliveryNote" json:"vendorDeliveryNote"`
	TrackingNumber     *string `db:"trackingNumber"     json:"trackingNumber"`
	Notes              *string `db:"notes"              json:"notes"`
	Status             string  `db:"status"             json:"status"`
	// Nullable — null means "the organization's own currency", same
	// convention as orders.currency/purchase_orders.currency. See
	// db/exchange_rate.go for the exchangeRate direction/storage convention;
	// this is what UpdateInboundDeliveryStatus uses to convert unitCost to
	// organization-currency terms before it reaches stockMovements.
	Currency         *string `db:"currency"         json:"currency"`
	ExchangeRate     *string `db:"exchangeRate"     json:"exchangeRate"`
	ExchangeRateDate *int64  `db:"exchangeRateDate" json:"exchangeRateDate"`
	CreatedAt        int64   `db:"createdAt"          json:"createdAt"`
	// Joined
	OrderNumber *string `db:"orderNumber" json:"orderNumber"`
	VendorName  *string `db:"vendorName"  json:"vendorName"`
}

type InboundDeliveryLineItem struct {
	ID                      string  `db:"id"                      json:"id"`
	DeliveryID              string  `db:"deliveryId"              json:"deliveryId"`
	PurchaseOrderLineItemID *string `db:"purchaseOrderLineItemId" json:"purchaseOrderLineItemId"`
	ProductID               *string `db:"productId"               json:"productId"`
	Description             string  `db:"description"             json:"description"`
	Quantity                float64 `db:"quantity"                json:"quantity"`
	UnitCost                *int64  `db:"unitCost"                json:"unitCost"`
	Unit                    *string `db:"unit"                    json:"unit"`
	Position                int     `db:"position"                json:"position"`
	// Joined from products via productId; nil when the line has no product
	// (free-text line) or the product isn't stock-tracked.
	StockEnabled *int     `db:"stockEnabled"  json:"stockEnabled"`
	CurrentStock *float64 `db:"currentStock"  json:"currentStock"`
	ProductName  *string  `db:"productName"   json:"productName"`
	Serialized   *int     `db:"serialized"    json:"serialized"`
}

type CreateInboundDeliveryLineItemRequest struct {
	PurchaseOrderLineItemID *string  `json:"purchaseOrderLineItemId"`
	ProductID               *string  `json:"productId"`
	Description             string   `json:"description"`
	Quantity                float64  `json:"quantity"`
	UnitCost                *float64 `json:"unitCost"` // cents sent from frontend
	Unit                    *string  `json:"unit"`
}

type CreateInboundDeliveryRequest struct {
	ID                 string                                 `json:"id"`
	OrganizationID     string                                 `json:"organizationId"`
	PurchaseOrderID    *string                                `json:"purchaseOrderId"`
	VendorID           *string                                `json:"vendorId"`
	DeliveryNumber     string                                 `json:"deliveryNumber"`
	DeliveryDate       int64                                  `json:"deliveryDate"`
	Currency           *string                                `json:"currency"`
	ExchangeRate       *float64                               `json:"exchangeRate"`
	ExchangeRateDate   *int64                                 `json:"exchangeRateDate"`
	VendorDeliveryNote *string                                `json:"vendorDeliveryNote"`
	TrackingNumber     *string                                `json:"trackingNumber"`
	Notes              *string                                `json:"notes"`
	LineItems          []CreateInboundDeliveryLineItemRequest `json:"lineItems"`
}

// UpdateInboundDeliveryRequest has no Status field — status changes go through
// PATCH only, so a PUT can't move stock by the back door.
type UpdateInboundDeliveryRequest struct {
	PurchaseOrderID    *string                                 `json:"purchaseOrderId"`
	VendorID           *string                                 `json:"vendorId"`
	DeliveryNumber     *string                                 `json:"deliveryNumber"`
	DeliveryDate       *int64                                  `json:"deliveryDate"`
	Currency           *string                                 `json:"currency"`
	ExchangeRate       *float64                                `json:"exchangeRate"`
	ExchangeRateDate   *int64                                  `json:"exchangeRateDate"`
	VendorDeliveryNote *string                                 `json:"vendorDeliveryNote"`
	TrackingNumber     *string                                 `json:"trackingNumber"`
	Notes              *string                                 `json:"notes"`
	LineItems          *[]CreateInboundDeliveryLineItemRequest `json:"lineItems"`
}

// inboundDeliveryStatusTransitions enumerates the only legal moves; "cancelled"
// is terminal. Mirrors INBOUND_DELIVERY_STATUS_TRANSITIONS in
// src/types/inbound-delivery.ts, enforced here too since that's client-side only.
var inboundDeliveryStatusTransitions = map[string]map[string]bool{
	"draft":    {"received": true, "cancelled": true},
	"received": {"cancelled": true},
}

const inboundDeliverySelect = `
	SELECT idl.*,
	       po.orderNumber AS orderNumber,
	       v.name AS vendorName
	FROM inbound_deliveries idl
	LEFT JOIN purchase_orders po ON idl.purchaseOrderId = po.id
	LEFT JOIN vendors v ON idl.vendorId = v.id`

func (d *Database) GetInboundDeliveries(organizationID string) ([]InboundDelivery, error) {
	rows := []InboundDelivery{}
	err := d.DB.Select(&rows, inboundDeliverySelect+`
		WHERE idl.organizationId = ?
		ORDER BY idl.deliveryDate DESC, idl.createdAt DESC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_inbound_deliveries: %w", err)
	}
	return rows, nil
}

func (d *Database) GetInboundDelivery(id string) (*InboundDelivery, error) {
	var row InboundDelivery
	err := d.DB.Get(&row, inboundDeliverySelect+` WHERE idl.id = ? LIMIT 1`, id)
	if err != nil {
		return nil, fmt.Errorf("get_inbound_delivery: %w", err)
	}
	return &row, nil
}

func (d *Database) GetInboundDeliveryLineItems(deliveryID string) ([]InboundDeliveryLineItem, error) {
	items := []InboundDeliveryLineItem{}
	err := d.DB.Select(&items, `
		SELECT dli.*,
		       p.stockEnabled AS stockEnabled,
		       p.stockQuantity AS currentStock,
		       p.name AS productName,
		       p.serialized AS serialized
		FROM inbound_delivery_line_items dli
		LEFT JOIN products p ON dli.productId = p.id
		WHERE dli.deliveryId = ?
		ORDER BY dli.position ASC`,
		deliveryID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_inbound_delivery_line_items: %w", err)
	}
	return items, nil
}

// GetPurchaseOrderReceivedQuantities sums received quantity per
// purchaseOrderLineItemId for the given purchase order — used to prefill a new
// receipt with only the outstanding quantity per line, and to show per-line
// fulfilment on the order.
//
// "Received" means status = 'received', matching what
// GetIncomingInvoiceMatch counts. A draft receipt has not moved any stock, so
// counting it here (via the looser status != 'cancelled') would make the order
// page report goods as received that the invoice matcher simultaneously
// reports as not received — the same quantity described two contradictory ways
// in two places.
//
// The sales-side mirror, GetOrderDeliveredQuantities, uses the looser test; it
// has no matching engine to contradict.
func (d *Database) GetPurchaseOrderReceivedQuantities(purchaseOrderID string) (map[string]float64, error) {
	rows := []struct {
		LineItemID string  `db:"purchaseOrderLineItemId"`
		Received   float64 `db:"received"`
	}{}
	err := d.DB.Select(&rows, `
		SELECT dli.purchaseOrderLineItemId AS purchaseOrderLineItemId,
		       SUM(dli.quantity) AS received
		FROM inbound_delivery_line_items dli
		JOIN inbound_deliveries idl ON dli.deliveryId = idl.id
		WHERE idl.purchaseOrderId = ?
		  AND idl.status = 'received'
		  AND dli.purchaseOrderLineItemId IS NOT NULL
		GROUP BY dli.purchaseOrderLineItemId`,
		purchaseOrderID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_purchase_order_received_quantities: %w", err)
	}

	result := make(map[string]float64, len(rows))
	for _, row := range rows {
		result[row.LineItemID] = row.Received
	}
	return result, nil
}

// NextInboundDeliveryNumber proposes the next "GR-%04d" (goods receipt) number,
// continuing from the highest in use rather than COUNT(*)+1. SUBSTR starts at 4
// because the prefix "GR-" is three characters.
func (d *Database) NextInboundDeliveryNumber(organizationID string) string {
	var maxNumber sql.NullInt64
	_ = d.DB.Get(&maxNumber, `
		SELECT MAX(CAST(SUBSTR(deliveryNumber, 4) AS INTEGER))
		FROM inbound_deliveries
		WHERE organizationId = ? AND deliveryNumber LIKE 'GR-%'`,
		organizationID,
	)
	return fmt.Sprintf("GR-%04d", maxNumber.Int64+1)
}

func (d *Database) CreateInboundDelivery(req CreateInboundDeliveryRequest) (*InboundDelivery, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}

	org, err := d.GetOrganization(req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("create_inbound_delivery organization: %w", err)
	}
	exchangeRate, err := resolveExchangeRateForSave(
		orgCurrencyOrDefault(org), "", nil, req.Currency, req.ExchangeRate,
	)
	if err != nil {
		return nil, err
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_inbound_delivery begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		INSERT INTO inbound_deliveries
		  (id, organizationId, purchaseOrderId, vendorId, deliveryNumber, deliveryDate,
		   currency, exchangeRate, exchangeRateDate,
		   vendorDeliveryNote, trackingNumber, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.PurchaseOrderID, req.VendorID, req.DeliveryNumber,
		req.DeliveryDate, req.Currency, exchangeRate, req.ExchangeRateDate,
		req.VendorDeliveryNote, req.TrackingNumber, req.Notes,
	)
	if err != nil {
		return nil, fmt.Errorf("create_inbound_delivery: %w", err)
	}
	if err := replaceInboundDeliveryLineItemsTx(tx, req.ID, req.LineItems); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_inbound_delivery commit: %w", err)
	}
	return d.GetInboundDelivery(req.ID)
}

func (d *Database) UpdateInboundDelivery(id string, req UpdateInboundDeliveryRequest) (*InboundDelivery, error) {
	current, err := d.GetInboundDelivery(id)
	if err != nil {
		return nil, fmt.Errorf("update_inbound_delivery lookup: %w", err)
	}
	if req.LineItems != nil && current.Status != "draft" {
		return nil, newValidationError("cannot edit line items of a %s goods receipt", current.Status)
	}
	// Once received, this receipt's stockMovements/average cost were already
	// converted using the rate in effect at that moment (see
	// UpdateInboundDeliveryStatus). Changing currency/exchangeRate afterwards
	// wouldn't retroactively fix those movements — it would just decouple the
	// header from what was actually posted.
	if (req.Currency != nil || req.ExchangeRate != nil) && current.Status != "draft" {
		return nil, newValidationError(
			"cannot change currency or exchange rate of a %s goods receipt", current.Status,
		)
	}

	// Same "only when touched" rule as the other document types: resolving/
	// validating the exchange rate needs the receipt's current currency+rate
	// only when this request is actually changing one of them.
	var exchangeRate *string
	exchangeRateSet := 0
	if req.Currency != nil || req.ExchangeRate != nil {
		org, err := d.GetOrganization(current.OrganizationID)
		if err != nil {
			return nil, fmt.Errorf("update_inbound_delivery organization: %w", err)
		}
		currentCurrency := ""
		if current.Currency != nil {
			currentCurrency = *current.Currency
		}
		rate, err := resolveExchangeRateForSave(
			orgCurrencyOrDefault(org), currentCurrency, current.ExchangeRate,
			req.Currency, req.ExchangeRate,
		)
		if err != nil {
			return nil, err
		}
		exchangeRate, exchangeRateSet = rate, 1
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_inbound_delivery begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Every field is COALESCE'd: a header-only edit (tracking number, notes)
	// must not blank the rest, and this endpoint is explicitly used for such
	// edits once a receipt is no longer a draft. exchangeRate/exchangeRateDate
	// are the exception — nullable, and only written when this request
	// actually touches currency/exchangeRate (see exchangeRateSet above).
	_, err = tx.Exec(`
		UPDATE inbound_deliveries SET
		  purchaseOrderId    = COALESCE(?, purchaseOrderId),
		  vendorId           = COALESCE(?, vendorId),
		  deliveryNumber     = COALESCE(?, deliveryNumber),
		  deliveryDate       = COALESCE(?, deliveryDate),
		  currency           = COALESCE(?, currency),
		  exchangeRate       = CASE WHEN ? THEN ? ELSE exchangeRate END,
		  exchangeRateDate   = CASE WHEN ? THEN ? ELSE exchangeRateDate END,
		  vendorDeliveryNote = COALESCE(?, vendorDeliveryNote),
		  trackingNumber     = COALESCE(?, trackingNumber),
		  notes              = COALESCE(?, notes)
		WHERE id = ?`,
		req.PurchaseOrderID, req.VendorID, req.DeliveryNumber, req.DeliveryDate, req.Currency,
		exchangeRateSet, exchangeRate,
		exchangeRateSet, req.ExchangeRateDate,
		req.VendorDeliveryNote, req.TrackingNumber, req.Notes,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("update_inbound_delivery: %w", err)
	}
	if req.LineItems != nil {
		if err := replaceInboundDeliveryLineItemsTx(tx, id, *req.LineItems); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_inbound_delivery commit: %w", err)
	}
	return d.GetInboundDelivery(id)
}

// inboundStockLine is a receipt line resolved to its stock-enabled product.
type inboundStockLine struct {
	LineItemID   string  `db:"lineItemId"`
	ProductID    string  `db:"productId"`
	ProductName  string  `db:"productName"`
	Quantity     float64 `db:"quantity"`
	UnitCost     *int64  `db:"unitCost"`
	CurrentStock float64 `db:"currentStock"`
	Serialized   int     `db:"serialized"`
	// Phase 7: which purchase order line this receipt line accrued GRNI
	// against, if any — used by the received->cancelled Finding B guard to
	// check whether that accrual has already been cleared by an approved bill.
	PurchaseOrderLineItemID *string `db:"purchaseOrderLineItemId"`
}

// getReceivableStockLines returns the receipt's line items linked to a
// stock-enabled product (picked directly or resolved from a purchase order
// line) — the only lines that affect inventory. Takes a generic execer (not
// *sqlx.Tx) so UpdateInboundDeliveryStatus can read it once via d.DB before
// its transaction opens — see that function's comment on why no d.DB read
// may happen once Beginx has run.
func getReceivableStockLines(exec sqlSelectExecer, deliveryID string) ([]inboundStockLine, error) {
	lines := []inboundStockLine{}
	err := exec.Select(&lines, `
		SELECT dli.id AS lineItemId,
		       p.id AS productId,
		       p.name AS productName,
		       dli.quantity AS quantity,
		       dli.unitCost AS unitCost,
		       p.stockQuantity AS currentStock,
		       p.serialized AS serialized,
		       dli.purchaseOrderLineItemId AS purchaseOrderLineItemId
		FROM inbound_delivery_line_items dli
		JOIN products p ON dli.productId = p.id
		WHERE dli.deliveryId = ? AND p.stockEnabled = 1`,
		deliveryID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_receivable_stock_lines: %w", err)
	}
	return lines, nil
}

// UpdateInboundDeliveryStatus updates a goods receipt's status, increasing
// inventory when a draft receipt is marked received and reversing that increase
// when an already-received receipt is cancelled.
//
// Receiving needs no availability check — stock is going up. Cancelling a
// received receipt does: if the goods have since been shipped out, reversing
// would drive stockQuantity negative, so every line is validated before any is
// written, the same two-pass shape as the outbound insufficient-stock guard.
//
// serialNumbers is keyed by line-item id and is required (exactly matching
// each line's quantity) for any line whose product is serialized — mandatory
// so the registry never falls behind actual stock movements. It's ignored
// for non-serialized lines and unused on any transition other than
// draft->received.
func (d *Database) UpdateInboundDeliveryStatus(id, status string, serialNumbers map[string][]string) (*InboundDelivery, error) {
	current, err := d.GetInboundDelivery(id)
	if err != nil {
		return nil, fmt.Errorf("update_inbound_delivery_status lookup: %w", err)
	}
	if status != current.Status && !inboundDeliveryStatusTransitions[current.Status][status] {
		return nil, newValidationError(
			"cannot transition goods receipt from %q to %q", current.Status, status,
		)
	}

	// Phase 7: every d.DB read either transition needs (the receipt's own
	// line items, its GRNI accrual/reversal lines, the organization's GL
	// defaults) happens here, before the transaction opens —
	// db.SetMaxOpenConns(1) means a second connection request while a tx
	// holds the only one blocks forever rather than erroring, so no d.DB
	// read may happen between Beginx and Commit below (see
	// UpdateInvoiceState's identical constraint). `lines`, once fetched, is
	// reused inside the transaction rather than re-queried via tx.Select —
	// nothing between this read and the transaction's writes can change a
	// receipt's own line items (this function is the only writer touching
	// this receipt's status).
	var lines []inboundStockLine
	var convertedUnitCosts map[string]int64
	var grniLines []CreateJournalLineRequest
	var grniJournal *Journal
	var existingGRNIEntry *JournalEntry

	switch {
	case current.Status == "draft" && status == "received":
		lines, err = getReceivableStockLines(d.DB, id)
		if err != nil {
			return nil, err
		}
		// unitCost on each line is in the receipt's own currency; convert to
		// organization-currency terms before it reaches stockMovements — the
		// weighted-average replay in product_cost.go has no per-currency
		// concept and would otherwise mix currencies into one "average".
		rate, err := parseExchangeRate(current.ExchangeRate)
		if err != nil {
			return nil, fmt.Errorf("update_inbound_delivery_status rate: %w", err)
		}
		convertedUnitCosts = map[string]int64{}
		for _, line := range lines {
			if line.UnitCost != nil {
				convertedUnitCosts[line.LineItemID] = convertCents(*line.UnitCost, rate)
			}
		}
		grniLines, grniJournal, err = buildReceiptGRNILines(d, current, lines, convertedUnitCosts)
		if err != nil {
			return nil, err
		}

	case current.Status == "received" && status == "cancelled":
		lines, err = getReceivableStockLines(d.DB, id)
		if err != nil {
			return nil, err
		}
		existingGRNIEntry, err = d.FindPostedEntryForSourceDocument("inbound_delivery", current.ID)
		if err != nil {
			return nil, err
		}
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_inbound_delivery_status begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	touched := []string{}

	switch {
	case current.Status == "draft" && status == "received":
		// Validate every serialized line before writing anything: whole-number
		// quantity, exactly that many (deduped) serials supplied, no serial
		// reused across lines of the same product in this one request, and no
		// serial that's already registered-and-in-stock (a genuinely new unit
		// or one returning from a prior ship-out are both fine).
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
		for productID, serials := range perProductSerials {
			if len(dedupeStrings(serials)) != len(serials) {
				return nil, newValidationError("duplicate serial number(s) supplied for the same product in this receipt")
			}
			lookups, err := lookupSerialNumbersTx(tx, productID, serials)
			if err != nil {
				return nil, err
			}
			for _, s := range serials {
				if lu, ok := lookups[s]; ok && lu.InStock {
					return nil, newValidationError("serial %q is already in stock", s)
				}
			}
		}

		for _, line := range lines {
			// Already converted to organization-currency terms above
			// (convertedUnitCosts), before the transaction opened.
			var unitCost *int64
			if v, ok := convertedUnitCosts[line.LineItemID]; ok {
				unitCost = &v
			}

			if line.Serialized == 1 {
				serials := dedupeStrings(serialNumbers[line.LineItemID])
				ids, err := getOrCreateSerialNumbersTx(tx, current.OrganizationID, line.ProductID, serials)
				if err != nil {
					return nil, err
				}
				for _, s := range serials {
					serialID := ids[s]
					movementID, _ := gonanoid.New()
					if err := insertStockMovementRowTx(tx, CreateStockMovementRequest{
						ID:               movementID,
						OrganizationID:   current.OrganizationID,
						ProductID:        line.ProductID,
						Type:             "in",
						Quantity:         1,
						UnitCost:         unitCost,
						Note:             ptrStr("Goods receipt " + current.DeliveryNumber),
						Reference:        &current.DeliveryNumber,
						SerialNumberID:   &serialID,
						SourceDocumentID: &current.ID,
					}); err != nil {
						return nil, fmt.Errorf("update_inbound_delivery_status add_stock: %w", err)
					}
				}
				if err := recomputeStockQuantityTx(tx, line.ProductID); err != nil {
					return nil, err
				}
				touched = append(touched, line.ProductID)
				continue
			}

			movementID, _ := gonanoid.New()
			if err := insertStockMovementTx(tx, CreateStockMovementRequest{
				ID:             movementID,
				OrganizationID: current.OrganizationID,
				ProductID:      line.ProductID,
				Type:           "in",
				Quantity:       line.Quantity,
				UnitCost:       unitCost,
				Note:           ptrStr("Goods receipt " + current.DeliveryNumber),
				Reference:      &current.DeliveryNumber,
			}); err != nil {
				return nil, fmt.Errorf("update_inbound_delivery_status add_stock: %w", err)
			}
			touched = append(touched, line.ProductID)
		}

		// Phase 7: accrue GRNI for whatever on this receipt actually has a
		// cost basis (grniLines built before the transaction opened, above).
		if grniLines != nil {
			if _, err := postAutoEntryTx(
				tx, current.OrganizationID, grniJournal.ID, "inbound_delivery", current.ID,
				current.DeliveryDate, current.DeliveryNumber, "Goods receipt "+current.DeliveryNumber, grniLines,
			); err != nil {
				return nil, err
			}
		}

	case current.Status == "received" && status == "cancelled":
		// Phase 7 / Finding B: cancelling a received receipt after its GRNI
		// accrual has already been cleared by an approved/paid bill would
		// reverse the wrong entry — the receipt's own Dr GRNI / Cr Inventory
		// reversal, while the bill's AP obligation and cost recognition both
		// still stand — leaving GRNI permanently unbalanced. Checked here,
		// before any stock reversal is written, the same fail-fast shape as
		// the stock-quantity guard below.
		for _, line := range lines {
			if line.PurchaseOrderLineItemID == nil {
				continue
			}
			cleared, err := grniClearedQtyForPOLine(tx, *line.PurchaseOrderLineItemID, "")
			if err != nil {
				return nil, err
			}
			if cleared > 0 {
				return nil, newValidationError(
					"cannot cancel: %q has already been billed against this receipt — void or cancel the bill first",
					line.ProductName,
				)
			}
		}

		// Serialized products dedupe by productId here (not per line) since
		// reversal looks up everything this receipt posted for that product
		// across all its lines combined, not line-by-line.
		serializedProductNames := map[string]string{}
		for _, line := range lines {
			if line.Serialized == 1 {
				serializedProductNames[line.ProductID] = line.ProductName
				continue
			}
			if line.Quantity > line.CurrentStock {
				return nil, newValidationError(
					"cannot cancel: %q has only %.2f in stock but this receipt added %.2f — "+
						"the goods have already been used or shipped",
					line.ProductName, line.CurrentStock, line.Quantity,
				)
			}
		}

		// For each serialized product, resolve exactly the serial units this
		// receipt posted — via sourceDocumentId, never `reference` (free text
		// a manual movement could coincidentally reuse) — and confirm every
		// one is still in stock before reversing any of them.
		postedByProduct := map[string][]string{}
		for productID, productName := range serializedProductNames {
			var posted []struct {
				SerialNumberID string `db:"serialNumberId"`
			}
			if err := tx.Select(&posted, `
				SELECT serialNumberId FROM stockMovements
				WHERE productId = ? AND sourceDocumentId = ? AND type = 'in' AND serialNumberId IS NOT NULL`,
				productID, current.ID,
			); err != nil {
				return nil, fmt.Errorf("update_inbound_delivery_status lookup_serials: %w", err)
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
				if !inStock[sid] {
					return nil, newValidationError(
						"cannot cancel: %q has a unit from this receipt that's already been used or shipped",
						productName,
					)
				}
			}
			postedByProduct[productID] = ids
		}

		for _, line := range lines {
			if line.Serialized == 1 {
				continue // reversed once per product below, not per line
			}
			movementID, _ := gonanoid.New()
			if err := insertStockMovementTx(tx, CreateStockMovementRequest{
				ID:             movementID,
				OrganizationID: current.OrganizationID,
				ProductID:      line.ProductID,
				Type:           "out",
				Quantity:       -line.Quantity,
				Note:           ptrStr("Goods receipt " + current.DeliveryNumber + " cancelled"),
				Reference:      &current.DeliveryNumber,
			}); err != nil {
				return nil, fmt.Errorf("update_inbound_delivery_status reverse_stock: %w", err)
			}
			touched = append(touched, line.ProductID)
		}
		for productID, ids := range postedByProduct {
			for _, sid := range ids {
				serialID := sid
				movementID, _ := gonanoid.New()
				if err := insertStockMovementRowTx(tx, CreateStockMovementRequest{
					ID:               movementID,
					OrganizationID:   current.OrganizationID,
					ProductID:        productID,
					Type:             "out",
					Quantity:         -1,
					Note:             ptrStr("Goods receipt " + current.DeliveryNumber + " cancelled"),
					Reference:        &current.DeliveryNumber,
					SerialNumberID:   &serialID,
					SourceDocumentID: &current.ID,
				}); err != nil {
					return nil, fmt.Errorf("update_inbound_delivery_status reverse_stock: %w", err)
				}
			}
			if err := recomputeStockQuantityTx(tx, productID); err != nil {
				return nil, err
			}
			touched = append(touched, productID)
		}

		// Phase 7: reverse the receipt's GRNI accrual, if it posted one (looked
		// up before the transaction opened, above) — an all-uncosted receipt
		// never did (buildReceiptGRNILines returns nil), and that's a no-op
		// here, not an error.
		if existingGRNIEntry != nil {
			if _, err := reverseEntryTx(tx, existingGRNIEntry, "goods receipt cancelled", current.DeliveryDate); err != nil {
				return nil, err
			}
		}
	}

	// Cost is derived from the movement history, so recompute after all of this
	// receipt's movements exist rather than once per line.
	for _, productID := range touched {
		if err := recomputeAverageCostTx(tx, productID); err != nil {
			return nil, fmt.Errorf("update_inbound_delivery_status recompute_cost: %w", err)
		}
	}

	if _, err := tx.Exec(`UPDATE inbound_deliveries SET status = ? WHERE id = ?`, status, id); err != nil {
		return nil, fmt.Errorf("update_inbound_delivery_status: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_inbound_delivery_status commit: %w", err)
	}
	return d.GetInboundDelivery(id)
}

func (d *Database) DeleteInboundDelivery(id string) (bool, error) {
	current, err := d.GetInboundDelivery(id)
	if err != nil {
		return false, fmt.Errorf("delete_inbound_delivery lookup: %w", err)
	}
	if current.Status == "received" {
		return false, newValidationError("cannot delete a received goods receipt — cancel it instead")
	}

	res, err := d.DB.Exec(`DELETE FROM inbound_deliveries WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete_inbound_delivery: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// replaceInboundDeliveryLineItemsTx clears and reinserts a receipt's line items,
// renumbering position from the slice order.
//
// productId is the only field that decides inventory impact, so when a line
// comes from a purchase order and doesn't name a product directly, both the
// product and the cost are resolved from that order line — the receipt then
// values the goods at what was ordered unless the user overrides it.
func replaceInboundDeliveryLineItemsTx(
	exec sqlGetExecer, deliveryID string, items []CreateInboundDeliveryLineItemRequest,
) error {
	if _, err := exec.Exec(
		`DELETE FROM inbound_delivery_line_items WHERE deliveryId = ?`, deliveryID,
	); err != nil {
		return fmt.Errorf("delete_inbound_delivery_line_items: %w", err)
	}

	for i, item := range items {
		id, _ := gonanoid.New()
		productID := item.ProductID
		var unitCost *int64
		if item.UnitCost != nil {
			cost := int64(*item.UnitCost)
			unitCost = &cost
		}

		if item.PurchaseOrderLineItemID != nil && (productID == nil || unitCost == nil) {
			var resolved struct {
				ProductID sql.NullString `db:"productId"`
				UnitPrice sql.NullInt64  `db:"unitPrice"`
			}
			if err := exec.Get(&resolved,
				`SELECT productId, unitPrice FROM purchase_order_line_items WHERE id = ?`,
				*item.PurchaseOrderLineItemID,
			); err == nil {
				if productID == nil && resolved.ProductID.Valid {
					productID = &resolved.ProductID.String
				}
				if unitCost == nil && resolved.UnitPrice.Valid {
					unitCost = &resolved.UnitPrice.Int64
				}
			}
		}

		if _, err := exec.Exec(`
			INSERT INTO inbound_delivery_line_items
			  (id, deliveryId, purchaseOrderLineItemId, productId, description, quantity, unitCost, unit, position)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, deliveryID, item.PurchaseOrderLineItemID, productID,
			item.Description, item.Quantity, unitCost, item.Unit, i,
		); err != nil {
			return fmt.Errorf("insert_inbound_delivery_line_item: %w", err)
		}
	}
	return nil
}
