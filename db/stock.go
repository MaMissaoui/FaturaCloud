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

// insertStockMovementTx inserts a movement and recomputes the product's stockQuantity.
// quantity must already be signed by the caller (+in, -out, ±adjustment delta).
func insertStockMovementTx(exec sqlExecer, req CreateStockMovementRequest) error {
	_, err := exec.Exec(
		`INSERT INTO stockMovements (id, organizationId, productId, type, quantity, unitCost, note, reference)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.ProductID, req.Type,
		req.Quantity, req.UnitCost, req.Note, req.Reference,
	)
	if err != nil {
		return fmt.Errorf("insert_stock_movement: %w", err)
	}

	_, err = exec.Exec(
		`UPDATE products SET stockQuantity = (
		   SELECT COALESCE(SUM(quantity), 0) FROM stockMovements WHERE productId = ?
		 ) WHERE id = ?`,
		req.ProductID, req.ProductID,
	)
	if err != nil {
		return fmt.Errorf("recompute_stock_quantity: %w", err)
	}
	return nil
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
	CreatedAt      *string `db:"createdAt"      json:"createdAt"`
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
	from := "FROM stockMovements sm JOIN products p ON sm.productId = p.id"
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

	query := "SELECT sm.* " + from + " " + where + " ORDER BY " + sortCol + " " + direction
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
		`SELECT * FROM stockMovements WHERE productId = ? ORDER BY createdAt DESC`,
		productID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_product_stock_movements: %w", err)
	}
	return movements, nil
}

// CreateStockMovement inserts a movement and recomputes the product's stockQuantity.
// quantity must already be signed by the caller (+in, -out, ±adjustment delta).
func (d *Database) CreateStockMovement(req CreateStockMovementRequest) (*StockMovement, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_stock_movement begin: %w", err)
	}
	defer tx.Rollback()

	if err := insertStockMovementTx(tx, req); err != nil {
		return nil, fmt.Errorf("create_stock_movement: %w", err)
	}
	// unitCost on a manual movement feeds the product's average cost, which is
	// always derived from the movement history rather than adjusted in place.
	if err := recomputeAverageCostTx(tx, req.ProductID); err != nil {
		return nil, fmt.Errorf("create_stock_movement: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_stock_movement commit: %w", err)
	}

	var m StockMovement
	if err = d.DB.Get(&m, `SELECT * FROM stockMovements WHERE id = ?`, req.ID); err != nil {
		return nil, fmt.Errorf("create_stock_movement fetch: %w", err)
	}
	return &m, nil
}

// DeleteStockMovement removes the movement and recomputes the product's stockQuantity.
func (d *Database) DeleteStockMovement(movementID string) (bool, error) {
	var productID string
	if err := d.DB.Get(&productID,
		`SELECT productId FROM stockMovements WHERE id = ?`, movementID,
	); err != nil {
		return false, fmt.Errorf("delete_stock_movement lookup: %w", err)
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

	_, err = tx.Exec(
		`UPDATE products SET stockQuantity = (
		   SELECT COALESCE(SUM(quantity), 0) FROM stockMovements WHERE productId = ?
		 ) WHERE id = ?`,
		productID, productID,
	)
	if err != nil {
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
