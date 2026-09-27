package db

import (
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// ProductFamily mirrors the product_families table (migration 0095) — a
// maintained, per-organization list of product groups ("Machine à laver",
// "Réfrigérateur", …) a product can belong to via products.familyId. It is
// deliberately NOT products.category, which only ever holds the
// finished/component classification the BOM/production and the sales/
// purchasing pickers depend on. Same shape as UnitOfMeasure without a
// default: unique name per organization, and deleting one is unconditional
// because products.familyId is ON DELETE SET NULL.
type ProductFamily struct {
	ID             string `db:"id"             json:"id"`
	OrganizationID string `db:"organizationId" json:"organizationId"`
	Name           string `db:"name"           json:"name"`
	CreatedAt      string `db:"createdAt"      json:"createdAt"`
}

type CreateProductFamilyRequest struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
}

type UpdateProductFamilyRequest struct {
	Name *string `json:"name"`
}

func (d *Database) GetProductFamilies(organizationID string) ([]ProductFamily, error) {
	families := []ProductFamily{}
	if err := d.DB.Select(&families,
		`SELECT * FROM product_families WHERE organizationId = ? ORDER BY name ASC`, organizationID,
	); err != nil {
		return nil, fmt.Errorf("get_product_families: %w", err)
	}
	return families, nil
}

func (d *Database) GetProductFamily(id string) (*ProductFamily, error) {
	var family ProductFamily
	if err := d.DB.Get(&family, `SELECT * FROM product_families WHERE id = ? LIMIT 1`, id); err != nil {
		return nil, fmt.Errorf("get_product_family: %w", err)
	}
	return &family, nil
}

func (d *Database) CreateProductFamily(req CreateProductFamilyRequest) (*ProductFamily, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, newValidationError("name is required")
	}
	if _, err := d.DB.Exec(
		`INSERT INTO product_families (id, organizationId, name) VALUES (?, ?, ?)`,
		req.ID, req.OrganizationID, req.Name,
	); err != nil {
		if isDuplicateProductFamilyName(err) {
			return nil, newValidationError("a product family named %q already exists", req.Name)
		}
		return nil, fmt.Errorf("create_product_family: %w", err)
	}
	return d.GetProductFamily(req.ID)
}

func (d *Database) UpdateProductFamily(id string, updates UpdateProductFamilyRequest) (*ProductFamily, error) {
	if updates.Name != nil {
		trimmed := strings.TrimSpace(*updates.Name)
		if trimmed == "" {
			return nil, newValidationError("name is required")
		}
		updates.Name = &trimmed
	}
	res, err := d.DB.Exec(`UPDATE product_families SET name = COALESCE(?, name) WHERE id = ?`, updates.Name, id)
	if err != nil {
		if isDuplicateProductFamilyName(err) {
			return nil, newValidationError("a product family named %q already exists", *updates.Name)
		}
		return nil, fmt.Errorf("update_product_family: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, newValidationError("product family not found")
	}
	return d.GetProductFamily(id)
}

// DeleteProductFamily is unconditional: products.familyId is ON DELETE SET
// NULL, so deleting an in-use family just ungroups its products.
func (d *Database) DeleteProductFamily(id string) (bool, error) {
	res, err := d.DB.Exec(`DELETE FROM product_families WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete_product_family: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func isDuplicateProductFamilyName(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed") &&
		strings.Contains(err.Error(), "product_families")
}
