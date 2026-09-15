package db

import (
	"database/sql"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// ProductionOrder consumes a finished product's Bill of Materials
// components (db/product_bom.go) and produces finished units — the
// assembly/production feature the BOM and Import serial-number range
// (migration 0073) were both built to support.
type ProductionOrder struct {
	ID             string `db:"id"             json:"id"`
	OrganizationID string `db:"organizationId" json:"organizationId"`
	OrderNumber    string `db:"orderNumber"    json:"orderNumber"`
	Status         string `db:"status"          json:"status"`
	// Nullable — see migration 0075's comment: deleting a product never
	// fails elsewhere in this app (every line-item table SET NULLs), and
	// this follows the same convention rather than inventing a stricter
	// rule. FinishedProductName is captured at creation so the order stays
	// readable even after its product is gone.
	FinishedProductID   *string `db:"finishedProductId"   json:"finishedProductId"`
	FinishedProductName string  `db:"finishedProductName" json:"finishedProductName"`
	Quantity            float64 `db:"quantity" json:"quantity"`
	Date                int64   `db:"date"      json:"date"`
	// No ON DELETE clause — guarded by DeleteImport, same precedent as
	// purchase_orders.importId.
	ImportID  *string `db:"importId" json:"importId"`
	Notes     *string `db:"notes"    json:"notes"`
	CreatedAt int64   `db:"createdAt" json:"createdAt"`
	// Joined
	Serialized   *int    `db:"serialized"   json:"serialized"`
	ImportNumber *string `db:"importNumber" json:"importNumber"`
}

// ProductionOrderComponentLine is one snapshotted BOM line — the recipe as
// it stood when the order was created, not a live reference (see
// ReplaceBillOfMaterials' comment on the same convention for BOM version
// lines). ComponentProductID/ComponentName follow the identical nullable +
// denormalized-name shape as ProductionOrder.FinishedProductID above.
type ProductionOrderComponentLine struct {
	ID                 string  `db:"id"                 json:"id"`
	ProductionOrderID  string  `db:"productionOrderId"  json:"productionOrderId"`
	ComponentProductID *string `db:"componentProductId" json:"componentProductId"`
	ComponentName      string  `db:"componentName"      json:"componentName"`
	QuantityPerUnit    float64 `db:"quantityPerUnit"    json:"quantityPerUnit"`
	TotalQuantity      float64 `db:"totalQuantity"      json:"totalQuantity"`
	CreatedAt          *string `db:"createdAt"          json:"createdAt"`
	// Joined — nil once the component product has been deleted.
	ComponentSKU  *string `db:"componentSku"  json:"componentSku"`
	ComponentUnit *string `db:"componentUnit" json:"componentUnit"`
}

type CreateProductionOrderRequest struct {
	ID                string  `json:"id"`
	OrganizationID    string  `json:"organizationId"`
	OrderNumber       string  `json:"orderNumber"`
	FinishedProductID string  `json:"finishedProductId"`
	Quantity          float64 `json:"quantity"`
	Date              int64   `json:"date"`
	ImportID          *string `json:"importId"`
	Notes             *string `json:"notes"`
}

// productionOrderStatusTransitions enumerates the only legal moves;
// "cancelled" is terminal — matching inbound/outbound delivery's reasoning
// (reversing a cancel would mean re-deriving stock/GL/serial state, not
// just flipping a status column), not purchase orders/orders' un-cancel.
// Mirrors PRODUCTION_ORDER_STATUS_TRANSITIONS in
// src/types/production-order.ts, enforced here too since that's
// client-side only.
var productionOrderStatusTransitions = map[string]map[string]bool{
	"draft":     {"completed": true, "cancelled": true},
	"completed": {"cancelled": true},
}

const productionOrderSelect = `
	SELECT po.*, p.serialized AS serialized, imp.importNumber AS importNumber
	FROM production_orders po
	LEFT JOIN products p ON po.finishedProductId = p.id
	LEFT JOIN imports imp ON po.importId = imp.id`

func (d *Database) GetProductionOrders(organizationID string) ([]ProductionOrder, error) {
	rows := []ProductionOrder{}
	err := d.DB.Select(&rows, productionOrderSelect+`
		WHERE po.organizationId = ?
		ORDER BY po.date DESC, po.createdAt DESC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_production_orders: %w", err)
	}
	return rows, nil
}

func (d *Database) GetProductionOrder(id string) (*ProductionOrder, error) {
	var row ProductionOrder
	err := d.DB.Get(&row, productionOrderSelect+` WHERE po.id = ? LIMIT 1`, id)
	if err != nil {
		return nil, fmt.Errorf("get_production_order: %w", err)
	}
	return &row, nil
}

func (d *Database) GetProductionOrderComponentLines(productionOrderID string) ([]ProductionOrderComponentLine, error) {
	lines := []ProductionOrderComponentLine{}
	err := d.DB.Select(&lines, `
		SELECT pcl.*, p.sku AS componentSku, p.unit AS componentUnit
		FROM production_order_component_lines pcl
		LEFT JOIN products p ON pcl.componentProductId = p.id
		WHERE pcl.productionOrderId = ?
		ORDER BY pcl.createdAt ASC, pcl.rowid ASC`,
		productionOrderID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_production_order_component_lines: %w", err)
	}
	return lines, nil
}

// NextProductionOrderNumber proposes the next "PRO-%04d" number, continuing
// from the highest in use rather than COUNT(*)+1 — same shape as
// NextPurchaseOrderNumber. SUBSTR starts at 5 because the prefix "PRO-" is
// four characters.
func (d *Database) NextProductionOrderNumber(organizationID string) string {
	var maxNumber sql.NullInt64
	_ = d.DB.Get(&maxNumber, `
		SELECT MAX(CAST(SUBSTR(orderNumber, 5) AS INTEGER))
		FROM production_orders
		WHERE organizationId = ? AND orderNumber LIKE 'PRO-%'`,
		organizationID,
	)
	return fmt.Sprintf("PRO-%04d", maxNumber.Int64+1)
}

// CreateProductionOrder snapshots the finished product's current BOM
// (GetBillOfMaterials) into this order's own component lines — a later BOM
// edit never retroactively changes an already-created order. Requires a
// non-empty BOM: an order for a recipe that doesn't exist yet has nothing
// to consume.
//
// Serialized components are rejected outright (not silently averaged) —
// consuming one as a single aggregate negative stockMovements row would
// leave product_serial_numbers reporting a unit "in stock" that was
// actually just consumed, since the schema invariant for a serialized
// product is one +-1 movement row per physical unit. Per-component serial
// capture on the consuming side is a real follow-up, not implemented here.
func (d *Database) CreateProductionOrder(req CreateProductionOrderRequest) (*ProductionOrder, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	if req.Quantity <= 0 {
		return nil, newValidationError("quantity must be greater than zero")
	}

	finished, err := d.GetProduct(req.FinishedProductID)
	if err != nil {
		return nil, newValidationError("finished product not found")
	}
	if err := requireSameOrg(req.OrganizationID, finished.OrganizationID, "finished product"); err != nil {
		return nil, err
	}
	if finished.Category == nil || *finished.Category != "finished" {
		return nil, newValidationError("a production order can only be created for a %q product", "finished")
	}
	// Every other stock-moving path in this app gates on stockEnabled at the
	// SQL level (getShippableStockLines, getReceivableStockLines). Production
	// orders did not, so a finished product with stock tracking switched off
	// still got real stockMovements rows, a non-zero stockQuantity and — on a
	// non-exact division — an Inventory GL entry, for a product the rest of
	// the app treats as outside inventory entirely (F73).
	if finished.StockEnabled != 1 {
		return nil, newValidationError(
			"%q does not have stock tracking enabled — a production order would have nothing to produce into",
			finished.Name,
		)
	}
	if finished.Serialized == 1 && req.Quantity != math.Trunc(req.Quantity) {
		return nil, newValidationError("%q is serialized — quantity must be a whole number", finished.Name)
	}

	if req.ImportID != nil && *req.ImportID != "" {
		imp, err := d.GetImport(*req.ImportID)
		if err != nil {
			return nil, newValidationError("import not found")
		}
		if err := requireSameOrg(req.OrganizationID, imp.OrganizationID, "import"); err != nil {
			return nil, err
		}
	}

	bom, err := d.GetBillOfMaterials(req.FinishedProductID)
	if err != nil {
		return nil, err
	}
	if len(bom) == 0 {
		return nil, newValidationError("%q has no bill of materials defined", finished.Name)
	}

	type resolvedLine struct {
		id                 string
		componentProductID string
		componentName      string
		quantityPerUnit    float64
		totalQuantity      float64
	}
	lines := make([]resolvedLine, 0, len(bom))
	for _, l := range bom {
		component, err := d.GetProduct(l.ComponentProductID)
		if err != nil {
			return nil, fmt.Errorf("create_production_order component: %w", err)
		}
		if component.Serialized == 1 {
			return nil, newValidationError(
				"cannot create: %q is a serialized component — serialized components can't be consumed by a production order",
				component.Name,
			)
		}
		id, _ := gonanoid.New()
		lines = append(lines, resolvedLine{
			id:                 id,
			componentProductID: l.ComponentProductID,
			componentName:      component.Name,
			quantityPerUnit:    l.QuantityPerUnit,
			totalQuantity:      roundBOMQuantity(l.QuantityPerUnit * req.Quantity),
		})
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_production_order begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`
		INSERT INTO production_orders
		  (id, organizationId, orderNumber, finishedProductId, finishedProductName, quantity, date, importId, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.OrderNumber, req.FinishedProductID, finished.Name,
		req.Quantity, req.Date, req.ImportID, req.Notes,
	); err != nil {
		return nil, fmt.Errorf("create_production_order: %w", err)
	}
	for _, line := range lines {
		if _, err := tx.Exec(`
			INSERT INTO production_order_component_lines
			  (id, productionOrderId, componentProductId, componentName, quantityPerUnit, totalQuantity)
			VALUES (?, ?, ?, ?, ?, ?)`,
			line.id, req.ID, line.componentProductID, line.componentName, line.quantityPerUnit, line.totalQuantity,
		); err != nil {
			return nil, fmt.Errorf("insert_production_order_component_line: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_production_order commit: %w", err)
	}
	return d.GetProductionOrder(req.ID)
}

// validateSerialAgainstImportRange checks a produced serial against the
// linked Import's reserved range (SerialNumberPrefix + [RangeStart,
// RangeEnd]) — a no-op when the import has no range configured, the same
// "absence is legal" convention as the range's own fields. The serial must
// start with the prefix (empty prefix matches everything) and the
// remaining suffix must parse as an integer within the range.
func validateSerialAgainstImportRange(imp *Import, serial string) error {
	if imp.SerialNumberRangeStart == nil || imp.SerialNumberRangeEnd == nil {
		return nil
	}
	prefix := ""
	if imp.SerialNumberPrefix != nil {
		prefix = *imp.SerialNumberPrefix
	}
	if !strings.HasPrefix(serial, prefix) {
		return newValidationError(
			"serial %q does not start with import %s's prefix %q", serial, imp.ImportNumber, prefix,
		)
	}
	suffix := strings.TrimPrefix(serial, prefix)
	n, err := strconv.ParseInt(suffix, 10, 64)
	if err != nil {
		return newValidationError(
			"serial %q has a non-numeric suffix after prefix %q", serial, prefix,
		)
	}
	if n < *imp.SerialNumberRangeStart || n > *imp.SerialNumberRangeEnd {
		return newValidationError(
			"serial %q is outside import %s's reserved range [%d, %d]",
			serial, imp.ImportNumber, *imp.SerialNumberRangeStart, *imp.SerialNumberRangeEnd,
		)
	}
	return nil
}

// UpdateProductionOrderStatus consumes BOM components and produces finished
// units when a draft order is marked completed, and reverses both when a
// completed order is cancelled.
//
// serialNumbers is required (exactly matching Quantity, no duplicates) when
// the finished product is serialized; ignored otherwise. When the order is
// linked to an Import with a serial-number range configured, every serial
// is also validated against that range — 409 naming the offending serial
// and the valid range otherwise.
func (d *Database) UpdateProductionOrderStatus(id, status string, serialNumbers []string) (*ProductionOrder, error) {
	current, err := d.GetProductionOrder(id)
	if err != nil {
		return nil, fmt.Errorf("update_production_order_status lookup: %w", err)
	}
	if status != current.Status && !productionOrderStatusTransitions[current.Status][status] {
		return nil, newValidationError(
			"cannot transition production order from %q to %q", current.Status, status,
		)
	}

	// Every d.DB read either transition needs happens here, before Beginx()
	// opens — db.SetMaxOpenConns(1) means a second connection request while
	// a tx holds the only one blocks forever (see
	// UpdateInboundDeliveryStatus's identical constraint). The GL residual
	// (see buildProductionOrderGLLines) is computed here too, entirely from
	// values already known pre-tx, so a missing-account 409 surfaces before
	// any stock is touched.
	var lines []ProductionOrderComponentLine
	var glLines []CreateJournalLineRequest
	var glJournal *Journal
	var perUnitCost int64
	var imp *Import
	var existingEntry *JournalEntry
	// Resolved once here, pre-tx, and reused inside the transaction below
	// instead of calling d.GetProduct again — a second d.DB request while
	// the transaction holds the only connection (db.SetMaxOpenConns(1))
	// would block forever, not error.
	var finished *Product

	switch {
	case current.Status == "draft" && status == "completed":
		if current.FinishedProductID == nil {
			return nil, newValidationError("cannot complete: the finished product no longer exists")
		}
		finished, err = d.GetProduct(*current.FinishedProductID)
		if err != nil {
			return nil, newValidationError("cannot complete: the finished product no longer exists")
		}
		lines, err = d.GetProductionOrderComponentLines(id)
		if err != nil {
			return nil, err
		}

		componentsCostTotal := new(big.Rat)
		for _, line := range lines {
			if line.ComponentProductID == nil {
				return nil, newValidationError(
					"cannot complete: component %q no longer exists", line.ComponentName,
				)
			}
			component, err := d.GetProduct(*line.ComponentProductID)
			if err != nil {
				return nil, fmt.Errorf("update_production_order_status component: %w", err)
			}
			// Defensive re-check: a component could have been toggled
			// serialized after this order was created (CreateProductionOrder
			// already rejects this at creation time).
			if component.Serialized == 1 {
				return nil, newValidationError(
					"cannot complete: %q is a serialized component — serialized components can't be consumed by a production order",
					component.Name,
				)
			}
			// Same insufficient-stock guard every other consuming document
			// type in this app enforces before writing anything (see
			// UpdateDeliveryStatus's identical check for shipping).
			if line.TotalQuantity > component.StockQuantity {
				return nil, newValidationError(
					"insufficient stock for %q: available %.2f, requested %.2f",
					component.Name, component.StockQuantity, line.TotalQuantity,
				)
			}
			unitCost, err := resolveMovementCost(d.DB, component, nil)
			if err != nil {
				return nil, wrapCostBasisError(err, "cannot complete production order")
			}
			qty, err := floatToRat(line.TotalQuantity)
			if err != nil {
				return nil, newValidationError("line for %q: invalid quantity", line.ComponentName)
			}
			componentsCostTotal.Add(componentsCostTotal, new(big.Rat).Mul(qty, new(big.Rat).SetInt64(unitCost)))
		}
		componentsCostTotalCents := roundHalfUp(componentsCostTotal, 0).Num().Int64()

		qtyRat, err := floatToRat(current.Quantity)
		if err != nil {
			return nil, newValidationError("invalid quantity")
		}
		perUnitCost = roundHalfUp(new(big.Rat).Quo(new(big.Rat).SetInt64(componentsCostTotalCents), qtyRat), 0).Num().Int64()
		producedValueCents := roundHalfUp(new(big.Rat).Mul(new(big.Rat).SetInt64(perUnitCost), qtyRat), 0).Num().Int64()

		glLines, glJournal, err = buildProductionOrderGLLines(d, current, componentsCostTotalCents, producedValueCents)
		if err != nil {
			return nil, err
		}

		if current.ImportID != nil {
			imp, err = d.GetImport(*current.ImportID)
			if err != nil {
				return nil, fmt.Errorf("update_production_order_status import: %w", err)
			}
		}

	case current.Status == "completed" && status == "cancelled":
		lines, err = d.GetProductionOrderComponentLines(id)
		if err != nil {
			return nil, err
		}
		if current.FinishedProductID != nil {
			finished, err = d.GetProduct(*current.FinishedProductID)
			if err != nil {
				return nil, newValidationError("cannot cancel: the finished product no longer exists")
			}
		}
		existingEntry, err = d.FindPostedEntryForSourceDocument("production_order", current.ID)
		if err != nil {
			return nil, err
		}
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_production_order_status begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// F48: re-check the status this whole function's decisions were made
	// against, now that this transaction actually holds the connection —
	// same guard as UpdateInboundDeliveryStatus/UpdateDeliveryStatus.
	var liveStatus string
	if err := tx.Get(&liveStatus, `SELECT status FROM production_orders WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("update_production_order_status status_check: %w", err)
	}
	if liveStatus != current.Status {
		return nil, newValidationError(
			"production order status changed to %q by another request — reload and try again", liveStatus,
		)
	}

	touched := []string{}

	switch {
	case current.Status == "draft" && status == "completed":
		var serials []string
		if finished.Serialized == 1 {
			if current.Quantity != math.Trunc(current.Quantity) {
				return nil, newValidationError("%q is serialized — quantity must be a whole number", finished.Name)
			}
			given := dedupeStrings(serialNumbers)
			if len(given) != int(current.Quantity) {
				return nil, newValidationError(
					"%q requires exactly %d serial number(s), got %d",
					finished.Name, int(current.Quantity), len(given),
				)
			}
			lookups, err := lookupSerialNumbersTx(tx, finished.ID, given)
			if err != nil {
				return nil, err
			}
			for _, s := range given {
				if lu, ok := lookups[s]; ok && lu.InStock {
					return nil, newValidationError("serial %q is already in stock", s)
				}
				if imp != nil {
					if err := validateSerialAgainstImportRange(imp, s); err != nil {
						return nil, err
					}
				}
			}
			serials = given
		}

		// Consume: one negative stockMovements row per component line.
		for _, line := range lines {
			movementID, _ := gonanoid.New()
			if err := insertStockMovementTx(tx, CreateStockMovementRequest{
				ID:               movementID,
				OrganizationID:   current.OrganizationID,
				ProductID:        *line.ComponentProductID,
				Type:             "out",
				Quantity:         -line.TotalQuantity,
				Note:             ptrStr("Production order " + current.OrderNumber),
				Reference:        &current.OrderNumber,
				SourceDocumentID: &current.ID,
			}); err != nil {
				return nil, fmt.Errorf("update_production_order_status consume: %w", err)
			}
			touched = append(touched, *line.ComponentProductID)
		}

		// Produce: finished units valued at perUnitCost (componentsCostTotal
		// / quantity, resolved before the transaction opened above).
		unitCostPtr := &perUnitCost
		if finished.Serialized == 1 {
			ids, err := getOrCreateSerialNumbersTx(tx, current.OrganizationID, finished.ID, serials)
			if err != nil {
				return nil, err
			}
			for _, s := range serials {
				serialID := ids[s]
				movementID, _ := gonanoid.New()
				if err := insertStockMovementRowTx(tx, CreateStockMovementRequest{
					ID:               movementID,
					OrganizationID:   current.OrganizationID,
					ProductID:        finished.ID,
					Type:             "in",
					Quantity:         1,
					UnitCost:         unitCostPtr,
					Note:             ptrStr("Production order " + current.OrderNumber),
					Reference:        &current.OrderNumber,
					SerialNumberID:   &serialID,
					SourceDocumentID: &current.ID,
				}); err != nil {
					return nil, fmt.Errorf("update_production_order_status produce: %w", err)
				}
			}
			if err := recomputeStockQuantityTx(tx, finished.ID); err != nil {
				return nil, err
			}
		} else {
			movementID, _ := gonanoid.New()
			if err := insertStockMovementTx(tx, CreateStockMovementRequest{
				ID:               movementID,
				OrganizationID:   current.OrganizationID,
				ProductID:        finished.ID,
				Type:             "in",
				Quantity:         current.Quantity,
				UnitCost:         unitCostPtr,
				Note:             ptrStr("Production order " + current.OrderNumber),
				Reference:        &current.OrderNumber,
				SourceDocumentID: &current.ID,
			}); err != nil {
				return nil, fmt.Errorf("update_production_order_status produce: %w", err)
			}
		}
		touched = append(touched, finished.ID)

		if glLines != nil {
			if _, err := postAutoEntryTx(
				tx, current.OrganizationID, glJournal.ID, "production_order", current.ID,
				current.Date, current.OrderNumber, "Production order "+current.OrderNumber, glLines,
			); err != nil {
				return nil, err
			}
		}

	case current.Status == "completed" && status == "cancelled":
		// Components go back in unconditionally — an inflow needs no
		// availability check, same reasoning as every other stock reversal
		// in this app.
		for _, line := range lines {
			if line.ComponentProductID == nil {
				continue // component since deleted — nothing to credit back to
			}
			movementID, _ := gonanoid.New()
			if err := insertStockMovementTx(tx, CreateStockMovementRequest{
				ID:               movementID,
				OrganizationID:   current.OrganizationID,
				ProductID:        *line.ComponentProductID,
				Type:             "in",
				Quantity:         line.TotalQuantity,
				Note:             ptrStr("Production order " + current.OrderNumber + " cancelled"),
				Reference:        &current.OrderNumber,
				SourceDocumentID: &current.ID,
			}); err != nil {
				return nil, fmt.Errorf("update_production_order_status reverse_consume: %w", err)
			}
			touched = append(touched, *line.ComponentProductID)
		}

		// Reversing the produced output DOES need an availability check —
		// the unit(s) may have since shipped out, the same guard
		// UpdateInboundDeliveryStatus's received->cancelled path uses.
		if finished != nil {
			if finished.Serialized == 1 {
				var posted []struct {
					SerialNumberID string `db:"serialNumberId"`
				}
				if err := tx.Select(&posted, `
					SELECT serialNumberId FROM stockMovements
					WHERE productId = ? AND sourceDocumentId = ? AND type = 'in' AND serialNumberId IS NOT NULL`,
					finished.ID, current.ID,
				); err != nil {
					return nil, fmt.Errorf("update_production_order_status lookup_serials: %w", err)
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
							"cannot cancel: %q has a unit from this order that's already been used or shipped",
							finished.Name,
						)
					}
				}
				for _, sid := range ids {
					serialID := sid
					movementID, _ := gonanoid.New()
					if err := insertStockMovementRowTx(tx, CreateStockMovementRequest{
						ID:               movementID,
						OrganizationID:   current.OrganizationID,
						ProductID:        finished.ID,
						Type:             "out",
						Quantity:         -1,
						Note:             ptrStr("Production order " + current.OrderNumber + " cancelled"),
						Reference:        &current.OrderNumber,
						SerialNumberID:   &serialID,
						SourceDocumentID: &current.ID,
					}); err != nil {
						return nil, fmt.Errorf("update_production_order_status reverse_produce: %w", err)
					}
				}
				if err := recomputeStockQuantityTx(tx, finished.ID); err != nil {
					return nil, err
				}
			} else {
				if current.Quantity > finished.StockQuantity {
					return nil, newValidationError(
						"cannot cancel: %q has only %.2f in stock but this order produced %.2f — "+
							"the goods have already been used or shipped",
						finished.Name, finished.StockQuantity, current.Quantity,
					)
				}
				movementID, _ := gonanoid.New()
				if err := insertStockMovementTx(tx, CreateStockMovementRequest{
					ID:               movementID,
					OrganizationID:   current.OrganizationID,
					ProductID:        finished.ID,
					Type:             "out",
					Quantity:         -current.Quantity,
					Note:             ptrStr("Production order " + current.OrderNumber + " cancelled"),
					Reference:        &current.OrderNumber,
					SourceDocumentID: &current.ID,
				}); err != nil {
					return nil, fmt.Errorf("update_production_order_status reverse_produce: %w", err)
				}
			}
			touched = append(touched, finished.ID)
		}

		if existingEntry != nil {
			if _, err := reverseEntryTx(tx, existingEntry, "production order cancelled", current.Date); err != nil {
				return nil, err
			}
		}
	}

	for _, productID := range touched {
		if err := recomputeAverageCostTx(tx, productID); err != nil {
			return nil, fmt.Errorf("update_production_order_status recompute_cost: %w", err)
		}
	}

	if _, err := tx.Exec(`UPDATE production_orders SET status = ? WHERE id = ?`, status, id); err != nil {
		return nil, fmt.Errorf("update_production_order_status: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_production_order_status commit: %w", err)
	}
	return d.GetProductionOrder(id)
}

func (d *Database) DeleteProductionOrder(id string) (bool, error) {
	current, err := d.GetProductionOrder(id)
	if err != nil {
		return false, fmt.Errorf("delete_production_order lookup: %w", err)
	}
	if current.Status != "draft" {
		// Named per status rather than "a %s ... — cancel it instead", which
		// rendered for an already-cancelled order as "cannot delete a
		// cancelled production order — cancel it instead" (F74).
		if current.Status == "cancelled" {
			return false, newValidationError("cannot delete a cancelled production order")
		}
		return false, newValidationError(
			"cannot delete a %s production order — cancel it instead", current.Status,
		)
	}

	// AND status = 'draft' with a RowsAffected check, not just the pre-check
	// read above: a delete racing a concurrent draft -> completed would
	// otherwise destroy a completed order, cascading away its component lines
	// while leaving the consume/produce stockMovements pointing at a
	// sourceDocumentId that no longer exists and any posted rounding-residual
	// entry orphaned and unreversible. Same guard shape as DeleteJournalEntry
	// (F48, F74).
	res, err := d.DB.Exec(`DELETE FROM production_orders WHERE id = ? AND status = 'draft'`, id)
	if err != nil {
		return false, fmt.Errorf("delete_production_order: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Either it was already gone, or it left draft between the read and
		// the delete. Re-read to tell those apart rather than reporting a
		// concurrent completion as a plain "not found".
		if latest, lookupErr := d.GetProductionOrder(id); lookupErr == nil && latest.Status != "draft" {
			return false, newValidationError(
				"cannot delete a %s production order — cancel it instead", latest.Status,
			)
		}
		return false, nil
	}
	return true, nil
}
