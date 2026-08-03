package db

import (
	"errors"
	"fmt"
	"math"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// ErrDuplicateSKU is returned by CreateProduct/UpdateProduct when the
// (organizationId, sku) unique index rejects the write.
var ErrDuplicateSKU = errors.New("product code already in use")

// Product mirrors the products table.
type Product struct {
	ID             string  `db:"id"             json:"id"`
	OrganizationID string  `db:"organizationId" json:"organizationId"`
	Name           string  `db:"name"           json:"name"`
	Description    *string `db:"description"    json:"description"`
	SKU            *string `db:"sku"            json:"sku"`
	Price          int64   `db:"price"          json:"price"`
	UnitCost       *int64  `db:"unitCost"       json:"unitCost"`
	Unit           *string `db:"unit"           json:"unit"`
	Type           string  `db:"type"           json:"type"`
	TaxRateID      *string `db:"taxRateId"      json:"taxRateId"`
	StockEnabled   int     `db:"stockEnabled"   json:"stockEnabled"`
	StockQuantity  float64 `db:"stockQuantity"  json:"stockQuantity"`
	// Serialized products track individual physical units (product_serial_numbers)
	// rather than a fungible quantity; only meaningful when StockEnabled == 1.
	// Toggling it is blocked (see UpdateProduct) while StockQuantity is non-zero
	// in either direction, since there's no principled way to either fabricate
	// serial identity for untracked legacy stock or reconcile already-serialized
	// stock against a suddenly-untracked quantity.
	Serialized int     `db:"serialized"    json:"serialized"`
	CreatedAt  *string `db:"createdAt"     json:"createdAt"`
}

type CreateProductRequest struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	Name           string  `json:"name"`
	Description    *string `json:"description"`
	SKU            *string `json:"sku"`
	Price          int64   `json:"price"`
	UnitCost       *int64  `json:"unitCost"`
	Unit           *string `json:"unit"`
	Type           string  `json:"type"`
	TaxRateID      *string `json:"taxRateId"`
	StockEnabled   int     `json:"stockEnabled"`
	Serialized     int     `json:"serialized"`
}

type UpdateProductRequest struct {
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	SKU          *string `json:"sku"`
	Price        int64   `json:"price"`
	UnitCost     *int64  `json:"unitCost"`
	Unit         *string `json:"unit"`
	Type         string  `json:"type"`
	TaxRateID    *string `json:"taxRateId"`
	StockEnabled int     `json:"stockEnabled"`
	Serialized   int     `json:"serialized"`
}

// ProductListOptions filters/pages/sorts GetProducts. Limit == 0 means "no
// limit" — the original full-fetch behavior every other caller (line-item
// product pickers on invoices/orders/deliveries/purchase-orders/
// inbound-deliveries) still relies on. SortField == "" keeps the original
// default order (name ascending).
type ProductListOptions struct {
	Search    string
	Limit     int
	Offset    int
	SortField string
	SortDesc  bool
}

// productSortColumns whitelists the columns the Products list page can sort
// by, mapped to the actual (possibly joined-table-qualified) SQL expression
// — never interpolate the field name from the request directly into ORDER
// BY. "taxRate" sorts by the tax rate's name, matching what the column
// displayed before this was server-side (the old client-side sorter resolved
// taxRateId to a name too, not the raw id).
var productSortColumns = map[string]string{
	"name":     "p.name",
	"type":     "p.type",
	"sku":      "p.sku",
	"price":    "p.price",
	"unitCost": "p.unitCost",
	"taxRate":  "tr.name",
	"stock":    "p.stockQuantity",
}

func (d *Database) GetProducts(organizationID string, opts ProductListOptions) ([]Product, int, error) {
	from := "FROM products p LEFT JOIN taxRates tr ON p.taxRateId = tr.id"
	where := "WHERE p.organizationId = ?"
	args := []any{organizationID}
	if opts.Search != "" {
		where += " AND (p.name LIKE ? OR p.sku LIKE ? OR p.description LIKE ? OR p.unit LIKE ? OR p.type LIKE ?)"
		like := "%" + opts.Search + "%"
		args = append(args, like, like, like, like, like)
	}

	var total int
	if err := d.DB.Get(&total, "SELECT COUNT(*) "+from+" "+where, args...); err != nil {
		return nil, 0, fmt.Errorf("get_products_count: %w", err)
	}

	sortCol, ok := productSortColumns[opts.SortField]
	if !ok {
		sortCol = "p.name"
	}
	direction := "ASC"
	if opts.SortDesc {
		direction = "DESC"
	}

	query := "SELECT p.* " + from + " " + where + " ORDER BY " + sortCol + " " + direction
	if opts.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, opts.Limit, opts.Offset)
	}

	products := []Product{}
	if err := d.DB.Select(&products, query, args...); err != nil {
		return nil, 0, fmt.Errorf("get_products: %w", err)
	}
	return products, total, nil
}

func (d *Database) GetProduct(productID string) (*Product, error) {
	var product Product
	err := d.DB.Get(&product,
		`SELECT * FROM products WHERE id = ? LIMIT 1`,
		productID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_product: %w", err)
	}
	return &product, nil
}

func (d *Database) CreateProduct(req CreateProductRequest) (*Product, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	if req.Type == "" {
		req.Type = "service"
	}
	if req.StockEnabled == 0 {
		req.Serialized = 0
	}
	// A brand-new product always has zero stock, so the toggle guard
	// UpdateProduct enforces has nothing to check here.
	_, err := d.DB.Exec(
		`INSERT INTO products (id, organizationId, name, description, sku, price, unitCost, unit, type, taxRateId, stockEnabled, serialized)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Name, req.Description, req.SKU,
		req.Price, req.UnitCost, req.Unit, req.Type, req.TaxRateID, req.StockEnabled, req.Serialized,
	)
	if err != nil {
		if isDuplicateSKU(err) {
			return nil, ErrDuplicateSKU
		}
		return nil, fmt.Errorf("create_product: %w", err)
	}
	return d.GetProduct(req.ID)
}

func (d *Database) UpdateProduct(productID string, updates UpdateProductRequest) (*Product, error) {
	if updates.Type == "" {
		updates.Type = "service"
	}
	if updates.StockEnabled == 0 {
		updates.Serialized = 0
	}

	current, err := d.GetProduct(productID)
	if err != nil {
		return nil, fmt.Errorf("update_product lookup: %w", err)
	}
	// Blocked in both directions: turning serial tracking ON while legacy
	// stock exists would fabricate identity for units that were never
	// actually serialized; turning it OFF while serialized stock exists
	// would strand the registry with no way to reconcile against a now-
	// untracked quantity. Tolerance, not `!= 0` — stockQuantity is a
	// SUM(REAL) and fractional movements are already supported, so exact
	// zero isn't guaranteed even when the "real" count is zero.
	if updates.Serialized != current.Serialized && math.Abs(current.StockQuantity) > 1e-9 {
		return nil, newValidationError(
			"cannot change serial number tracking for %q while stock is non-zero (%.2f) — adjust stock to zero first",
			current.Name, current.StockQuantity,
		)
	}

	_, err = d.DB.Exec(
		`UPDATE products
		 SET name = ?, description = ?, sku = ?, price = ?, unitCost = ?, unit = ?, type = ?, taxRateId = ?, stockEnabled = ?, serialized = ?
		 WHERE id = ?`,
		updates.Name, updates.Description, updates.SKU, updates.Price,
		updates.UnitCost, updates.Unit, updates.Type, updates.TaxRateID, updates.StockEnabled, updates.Serialized,
		productID,
	)
	if err != nil {
		if isDuplicateSKU(err) {
			return nil, ErrDuplicateSKU
		}
		return nil, fmt.Errorf("update_product: %w", err)
	}
	return d.GetProduct(productID)
}

// isDuplicateSKU recognizes the raw SQLite unique-index violation on
// (organizationId, sku).
func isDuplicateSKU(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "sku")
}

func (d *Database) DeleteProduct(productID string) (bool, error) {
	res, err := d.DB.Exec(`DELETE FROM products WHERE id = ?`, productID)
	if err != nil {
		return false, fmt.Errorf("delete_product: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
