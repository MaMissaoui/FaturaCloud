package db

import (
	"database/sql"
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// sqlExecer is satisfied by both *sqlx.DB and *sqlx.Tx, letting insertStockMovementTx
// run either standalone or as part of a caller's larger transaction.
type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// insertStockMovementRowTx inserts a single movement row without recomputing
// stockQuantity — split out from insertStockMovementTx so a serialized
// product's fan-out (one row per unit) can insert N rows and recompute once,
// instead of once per row. quantity must already be signed by the caller
// (+in, -out, ±adjustment delta; always exactly ±1 for a serialized unit).
func insertStockMovementRowTx(exec sqlExecer, req CreateStockMovementRequest) error {
	_, err := exec.Exec(
		`INSERT INTO stockMovements (id, organizationId, productId, type, quantity, unitCost, note, reference, serialNumberId, sourceDocumentId)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.ProductID, req.Type,
		req.Quantity, req.UnitCost, req.Note, req.Reference, req.SerialNumberID, req.SourceDocumentID,
	)
	if err != nil {
		return fmt.Errorf("insert_stock_movement: %w", err)
	}
	return nil
}

// recomputeStockQuantityTx re-derives products.stockQuantity as
// SUM(quantity) over all of a product's movements — never adjusted in
// place, so repeated recomputation can never drift.
func recomputeStockQuantityTx(exec sqlExecer, productID string) error {
	_, err := exec.Exec(
		`UPDATE products SET stockQuantity = (
		   SELECT COALESCE(SUM(quantity), 0) FROM stockMovements WHERE productId = ?
		 ) WHERE id = ?`,
		productID, productID,
	)
	if err != nil {
		return fmt.Errorf("recompute_stock_quantity: %w", err)
	}
	return nil
}

// insertStockMovementTx inserts a single movement and recomputes the
// product's stockQuantity. quantity must already be signed by the caller
// (+in, -out, ±adjustment delta).
func insertStockMovementTx(exec sqlExecer, req CreateStockMovementRequest) error {
	if err := insertStockMovementRowTx(exec, req); err != nil {
		return err
	}
	return recomputeStockQuantityTx(exec, req.ProductID)
}

// StockMovement mirrors the stockMovements table.
// quantity is a signed delta: positive = stock increase, negative = stock decrease.
// stockQuantity on the product is always SUM(quantity) over all its movements.
type StockMovement struct {
	ID             string  `db:"id"             json:"id"`
	OrganizationID string  `db:"organizationId" json:"organizationId"`
	ProductID      string  `db:"productId"      json:"productId"`
	Type           string  `db:"type"           json:"type"`
	Quantity       float64 `db:"quantity"       json:"quantity"`
	UnitCost       *int64  `db:"unitCost"       json:"unitCost"`
	Note           *string `db:"note"           json:"note"`
	Reference      *string `db:"reference"      json:"reference"`
	// Set only for a serialized product, where this row represents exactly
	// one physical unit (Quantity is always ±1). SourceDocumentID is the
	// receiving/shipping document's own row id (not Reference, which is
	// free text) — cancel-reversal for serialized lines looks up exactly
	// the rows a document posted by this column.
	SerialNumberID   *string `db:"serialNumberId"   json:"serialNumberId"`
	SourceDocumentID *string `db:"sourceDocumentId" json:"sourceDocumentId"`
	CreatedAt        *string `db:"createdAt"        json:"createdAt"`
	// Joined, for the Inventory ledger UI.
	SerialNumberValue *string `db:"serialNumberValue" json:"serialNumberValue"`
}

type CreateStockMovementRequest struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	ProductID      string  `json:"productId"`
	Type           string  `json:"type"`
	Quantity       float64 `json:"quantity"`
	UnitCost       *int64  `json:"unitCost"`
	Note           *string `json:"note"`
	Reference      *string `json:"reference"`
	// SerialNumbers is the public request field for a serialized product's
	// manual movement (POST /api/stock-movements): new/returning serials
	// for "in", existing in-stock serials to remove for "out". Quantity is
	// ignored for a serialized product — it's derived as len(SerialNumbers).
	SerialNumbers []string `json:"serialNumbers"`
	// SerialNumberID/SourceDocumentID are set internally by the fan-out
	// logic (CreateStockMovement, UpdateInboundDeliveryStatus,
	// UpdateDeliveryStatus) — never accepted directly from a client.
	SerialNumberID   *string `json:"-"`
	SourceDocumentID *string `json:"-"`
}

// StockMovementListOptions filters/pages/sorts GetStockMovements. Limit == 0
// means "no limit", ProductID == "" means unfiltered — both preserve the
// original full-fetch behavior. SortField == "" keeps the original default
// order (createdAt descending).
type StockMovementListOptions struct {
	ProductID string
	Limit     int
	Offset    int
	SortField string
	SortDesc  bool
}

// stockMovementSortColumns whitelists Inventory's sortable columns. "product"
// sorts by the linked product's name (matching the old client-side sorter,
// which resolved productId to a name via lookup, not the raw id) — safe as
// an INNER JOIN since stockMovements.productId is ON DELETE CASCADE, so
// every movement always has a product.
var stockMovementSortColumns = map[string]string{
	"date":      "sm.createdAt",
	"product":   "p.name",
	"type":      "sm.type",
	"quantity":  "sm.quantity",
	"reference": "sm.reference",
	"note":      "sm.note",
}

func (d *Database) GetStockMovements(organizationID string, opts StockMovementListOptions) ([]StockMovement, int, error) {
	from := "FROM stockMovements sm JOIN products p ON sm.productId = p.id " +
		"LEFT JOIN product_serial_numbers sn ON sm.serialNumberId = sn.id"
	where := "WHERE sm.organizationId = ?"
	args := []any{organizationID}
	if opts.ProductID != "" {
		where += " AND sm.productId = ?"
		args = append(args, opts.ProductID)
	}

	var total int
	if err := d.DB.Get(&total, "SELECT COUNT(*) "+from+" "+where, args...); err != nil {
		return nil, 0, fmt.Errorf("get_stock_movements_count: %w", err)
	}

	// createdAt DESC (newest first) is the original default — SortDesc's zero
	// value (false/ascending) only applies once a field is actually requested.
	sortField, sortDesc := opts.SortField, opts.SortDesc
	if sortField == "" {
		sortField, sortDesc = "date", true
	}
	sortCol, ok := stockMovementSortColumns[sortField]
	if !ok {
		sortCol = "sm.createdAt"
	}
	direction := "ASC"
	if sortDesc {
		direction = "DESC"
	}

	query := "SELECT sm.*, sn.serialNumber AS serialNumberValue " + from + " " + where + " ORDER BY " + sortCol + " " + direction
	if opts.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, opts.Limit, opts.Offset)
	}

	movements := []StockMovement{}
	if err := d.DB.Select(&movements, query, args...); err != nil {
		return nil, 0, fmt.Errorf("get_stock_movements: %w", err)
	}
	return movements, total, nil
}

func (d *Database) GetProductStockMovements(productID string) ([]StockMovement, error) {
	movements := []StockMovement{}
	err := d.DB.Select(&movements,
		`SELECT sm.*, sn.serialNumber AS serialNumberValue
		 FROM stockMovements sm
		 LEFT JOIN product_serial_numbers sn ON sm.serialNumberId = sn.id
		 WHERE sm.productId = ? ORDER BY sm.createdAt DESC`,
		productID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_product_stock_movements: %w", err)
	}
	return movements, nil
}

// CreateStockMovementResult carries every movement row a request produced
// (more than one for a serialized product's fan-out) plus the refreshed
// product — a serialized request can post many rows, so a per-row
// stockQuantity delta the caller could apply client-side no longer exists;
// the authoritative post-write product is simpler than a conditional shape.
type CreateStockMovementResult struct {
	Movements []StockMovement `json:"movements"`
	Product   *Product        `json:"product"`
}

// CreateStockMovement inserts a movement and recomputes the product's
// stockQuantity. For a non-serialized product, quantity must already be
// signed by the caller (+in, -out, ±adjustment delta) and exactly one
// movement row results. For a serialized product, only "in"/"out" are
// accepted, SerialNumbers is required, and one ±1 row is posted per serial
// — "in" registers new/returning serials, "out" requires each serial to
// already be in stock.
func (d *Database) CreateStockMovement(req CreateStockMovementRequest) (*CreateStockMovementResult, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}

	product, err := d.GetProduct(req.ProductID)
	if err != nil {
		return nil, fmt.Errorf("create_stock_movement product: %w", err)
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_stock_movement begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var movementIDs []string

	if product.Serialized == 1 {
		if req.Type != "in" && req.Type != "out" {
			return nil, newValidationError(
				"serialized products only support stock in/out movements, not %q — "+
					"adjustment/count movements have no per-unit identity to apply to",
				req.Type,
			)
		}
		serials := dedupeStrings(req.SerialNumbers)
		if len(serials) == 0 {
			return nil, newValidationError("at least one serial number is required for a serialized product")
		}

		lookups, err := lookupSerialNumbersTx(tx, req.ProductID, serials)
		if err != nil {
			return nil, err
		}

		if req.Type == "in" {
			for _, s := range serials {
				if lu, ok := lookups[s]; ok && lu.InStock {
					return nil, newValidationError("serial %q is already in stock for %q", s, product.Name)
				}
			}
			ids, err := getOrCreateSerialNumbersTx(tx, req.OrganizationID, req.ProductID, serials)
			if err != nil {
				return nil, err
			}
			for _, s := range serials {
				serialID := ids[s]
				movementID, _ := gonanoid.New()
				if err := insertStockMovementRowTx(tx, CreateStockMovementRequest{
					ID: movementID, OrganizationID: req.OrganizationID, ProductID: req.ProductID,
					Type: "in", Quantity: 1, UnitCost: req.UnitCost, Note: req.Note, Reference: req.Reference,
					SerialNumberID: &serialID,
				}); err != nil {
					return nil, fmt.Errorf("create_stock_movement insert: %w", err)
				}
				movementIDs = append(movementIDs, movementID)
			}
		} else { // "out"
			for _, s := range serials {
				lu, ok := lookups[s]
				if !ok || !lu.InStock {
					return nil, newValidationError("serial %q is not currently in stock for %q", s, product.Name)
				}
			}
			for _, s := range serials {
				serialID := lookups[s].ID
				movementID, _ := gonanoid.New()
				if err := insertStockMovementRowTx(tx, CreateStockMovementRequest{
					ID: movementID, OrganizationID: req.OrganizationID, ProductID: req.ProductID,
					Type: "out", Quantity: -1, Note: req.Note, Reference: req.Reference,
					SerialNumberID: &serialID,
				}); err != nil {
					return nil, fmt.Errorf("create_stock_movement insert: %w", err)
				}
				movementIDs = append(movementIDs, movementID)
			}
		}

		if err := recomputeStockQuantityTx(tx, req.ProductID); err != nil {
			return nil, err
		}
	} else {
		if err := insertStockMovementTx(tx, req); err != nil {
			return nil, fmt.Errorf("create_stock_movement: %w", err)
		}
		movementIDs = append(movementIDs, req.ID)
	}

	// unitCost on a costed inflow feeds the product's average cost, which is
	// always derived from the movement history rather than adjusted in place.
	if err := recomputeAverageCostTx(tx, req.ProductID); err != nil {
		return nil, fmt.Errorf("create_stock_movement: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_stock_movement commit: %w", err)
	}

	movements := make([]StockMovement, 0, len(movementIDs))
	for _, id := range movementIDs {
		var m StockMovement
		if err := d.DB.Get(&m, `SELECT * FROM stockMovements WHERE id = ?`, id); err != nil {
			return nil, fmt.Errorf("create_stock_movement fetch: %w", err)
		}
		movements = append(movements, m)
	}
	refreshedProduct, err := d.GetProduct(req.ProductID)
	if err != nil {
		return nil, fmt.Errorf("create_stock_movement product_fetch: %w", err)
	}
	return &CreateStockMovementResult{Movements: movements, Product: refreshedProduct}, nil
}

// DeleteStockMovement removes the movement and recomputes the product's stockQuantity.
// Movements generated by a shipped/delivered outbound delivery or a received
// inbound delivery are guarded the same way deleting that document itself is
// (DeleteDelivery, DeleteInboundDelivery) — cancelling those documents already
// reverses their stock impact correctly; deleting the movement directly would
// desync stockQuantity/average cost from a document that still claims it happened.
func (d *Database) DeleteStockMovement(movementID string) (bool, error) {
	var m struct {
		ProductID string  `db:"productId"`
		Reference *string `db:"reference"`
	}
	if err := d.DB.Get(&m,
		`SELECT productId, reference FROM stockMovements WHERE id = ?`, movementID,
	); err != nil {
		return false, fmt.Errorf("delete_stock_movement lookup: %w", err)
	}
	productID := m.ProductID

	if m.Reference != nil {
		var deliveryStatus sql.NullString
		if err := d.DB.Get(&deliveryStatus,
			`SELECT status FROM outbound_deliveries WHERE deliveryNumber = ?`, *m.Reference,
		); err != nil && err != sql.ErrNoRows {
			return false, fmt.Errorf("delete_stock_movement check_delivery: %w", err)
		}
		if deliveryStatus.Valid && (deliveryStatus.String == "shipped" || deliveryStatus.String == "delivered") {
			return false, newValidationError(
				"cannot delete a movement generated by delivery %s — cancel the delivery instead",
				*m.Reference,
			)
		}

		var receiptStatus sql.NullString
		if err := d.DB.Get(&receiptStatus,
			`SELECT status FROM inbound_deliveries WHERE deliveryNumber = ?`, *m.Reference,
		); err != nil && err != sql.ErrNoRows {
			return false, fmt.Errorf("delete_stock_movement check_receipt: %w", err)
		}
		if receiptStatus.Valid && receiptStatus.String == "received" {
			return false, newValidationError(
				"cannot delete a movement generated by goods receipt %s — cancel the receipt instead",
				*m.Reference,
			)
		}
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return false, fmt.Errorf("delete_stock_movement begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`DELETE FROM stockMovements WHERE id = ?`, movementID)
	if err != nil {
		return false, fmt.Errorf("delete_stock_movement delete: %w", err)
	}

	if err := recomputeStockQuantityTx(tx, productID); err != nil {
		return false, fmt.Errorf("delete_stock_movement recompute: %w", err)
	}
	if err := recomputeAverageCostTx(tx, productID); err != nil {
		return false, fmt.Errorf("delete_stock_movement recompute_cost: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("delete_stock_movement commit: %w", err)
	}

	n, _ := res.RowsAffected()
	return n > 0, nil
}
