package db

import (
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

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

func (d *Database) GetStockMovements(organizationID string) ([]StockMovement, error) {
	movements := []StockMovement{}
	err := d.DB.Select(&movements,
		`SELECT * FROM stockMovements WHERE organizationId = ? ORDER BY createdAt DESC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_stock_movements: %w", err)
	}
	return movements, nil
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

	_, err = tx.Exec(
		`INSERT INTO stockMovements (id, organizationId, productId, type, quantity, unitCost, note, reference)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.ProductID, req.Type,
		req.Quantity, req.UnitCost, req.Note, req.Reference,
	)
	if err != nil {
		return nil, fmt.Errorf("create_stock_movement insert: %w", err)
	}

	_, err = tx.Exec(
		`UPDATE products SET stockQuantity = (
		   SELECT COALESCE(SUM(quantity), 0) FROM stockMovements WHERE productId = ?
		 ) WHERE id = ?`,
		req.ProductID, req.ProductID,
	)
	if err != nil {
		return nil, fmt.Errorf("create_stock_movement recompute: %w", err)
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

	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("delete_stock_movement commit: %w", err)
	}

	n, _ := res.RowsAffected()
	return n > 0, nil
}
