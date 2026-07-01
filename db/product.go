package db

import (
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

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
	CreatedAt      *string `db:"createdAt"      json:"createdAt"`
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
}

func (d *Database) GetProducts(organizationID string) ([]Product, error) {
	products := []Product{}
	err := d.DB.Select(&products,
		`SELECT * FROM products WHERE organizationId = ? ORDER BY name ASC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_products: %w", err)
	}
	return products, nil
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
	_, err := d.DB.Exec(
		`INSERT INTO products (id, organizationId, name, description, sku, price, unitCost, unit, type, taxRateId, stockEnabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Name, req.Description, req.SKU,
		req.Price, req.UnitCost, req.Unit, req.Type, req.TaxRateID, req.StockEnabled,
	)
	if err != nil {
		return nil, fmt.Errorf("create_product: %w", friendlyProductError(err))
	}
	return d.GetProduct(req.ID)
}

func (d *Database) UpdateProduct(productID string, updates UpdateProductRequest) (*Product, error) {
	if updates.Type == "" {
		updates.Type = "service"
	}
	_, err := d.DB.Exec(
		`UPDATE products
		 SET name = ?, description = ?, sku = ?, price = ?, unitCost = ?, unit = ?, type = ?, taxRateId = ?, stockEnabled = ?
		 WHERE id = ?`,
		updates.Name, updates.Description, updates.SKU, updates.Price,
		updates.UnitCost, updates.Unit, updates.Type, updates.TaxRateID, updates.StockEnabled,
		productID,
	)
	if err != nil {
		return nil, fmt.Errorf("update_product: %w", friendlyProductError(err))
	}
	return d.GetProduct(productID)
}

// friendlyProductError turns the raw SQLite unique-index violation on
// (organizationId, sku) into a message a user can act on.
func friendlyProductError(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "sku") {
		return fmt.Errorf("product code already in use")
	}
	return err
}

func (d *Database) DeleteProduct(productID string) (bool, error) {
	res, err := d.DB.Exec(`DELETE FROM products WHERE id = ?`, productID)
	if err != nil {
		return false, fmt.Errorf("delete_product: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
