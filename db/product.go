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
	// Unit is a legacy free-text label, kept for backward compatibility with
	// any caller that never adopted UnitOfMeasureID (cmd/seed-demo, a direct
	// API client) and as the display value every existing reader of this
	// column (Inventory, Products list, BOM component display) still shows
	// with no changes of its own — see resolveProductUnit's comment. Once
	// UnitOfMeasureID is set, this column is server-derived from it, not
	// independently editable.
	Unit *string `db:"unit" json:"unit"`
	// UnitOfMeasureID is the structured Base Unit of Measure (units_of_measure,
	// migration 0076) — nil means the product still uses free-text Unit only
	// (legacy data, or a caller that hasn't adopted the structured list).
	UnitOfMeasureID *string `db:"unitOfMeasureId" json:"unitOfMeasureId"`
	Type            string  `db:"type"           json:"type"`
	TaxRateID       *string `db:"taxRateId"      json:"taxRateId"`
	StockEnabled    int     `db:"stockEnabled"   json:"stockEnabled"`
	StockQuantity   float64 `db:"stockQuantity"  json:"stockQuantity"`
	// Serialized products track individual physical units (product_serial_numbers)
	// rather than a fungible quantity; only meaningful when StockEnabled == 1.
	// Toggling it is blocked (see UpdateProduct) while StockQuantity is non-zero
	// in either direction, since there's no principled way to either fabricate
	// serial identity for untracked legacy stock or reconcile already-serialized
	// stock against a suddenly-untracked quantity.
	Serialized int     `db:"serialized"    json:"serialized"`
	CreatedAt  *string `db:"createdAt"     json:"createdAt"`

	// Category distinguishes a purchasable component/intermediate from a
	// sellable finished good — orthogonal to Type, since it only ever
	// applies to a physical product, never a service (see CreateProduct/
	// UpdateProduct, which force it to nil whenever Type != "product", the
	// same defensive-clear shape Serialized already gets when StockEnabled
	// is off). nil means "unclassified" — every product picker treats that
	// as eligible on both the purchasing and sales side, so this stays
	// fully backward compatible and opt-in.
	Category *string `db:"category" json:"category"`

	// FamilyID groups the product under a maintained product family
	// (product_families, migration 0095, db/product_family.go) — separate
	// from Category. nil means no family.
	FamilyID *string `db:"familyId" json:"familyId"`

	// Per-product override of organizations.defaultRevenueAccountId /
	// defaultExpenseAccountId — same override shape as TaxRateID overriding
	// taxRates.isDefault. Nil means "use the organization's default".
	RevenueAccountID *string `db:"revenueAccountId" json:"revenueAccountId"`
	ExpenseAccountID *string `db:"expenseAccountId" json:"expenseAccountId"`

	// IsArchived retires a product that has history and so can't be deleted
	// (migration 0105, audit F167): the pickers and, by default, the
	// Products list leave it out, while its documents and stock reports keep
	// it. A new product is never archived.
	IsArchived bool `db:"isArchived" json:"isArchived"`
}

type CreateProductRequest struct {
	ID               string  `json:"id"`
	OrganizationID   string  `json:"organizationId"`
	Name             string  `json:"name"`
	Description      *string `json:"description"`
	SKU              *string `json:"sku"`
	Price            int64   `json:"price"`
	UnitCost         *int64  `json:"unitCost"`
	Unit             *string `json:"unit"`
	UnitOfMeasureID  *string `json:"unitOfMeasureId"`
	Type             string  `json:"type"`
	Category         *string `json:"category"`
	TaxRateID        *string `json:"taxRateId"`
	StockEnabled     int     `json:"stockEnabled"`
	Serialized       int     `json:"serialized"`
	RevenueAccountID *string `json:"revenueAccountId"`
	ExpenseAccountID *string `json:"expenseAccountId"`
	FamilyID         *string `json:"familyId"`
}

type UpdateProductRequest struct {
	Name             string  `json:"name"`
	Description      *string `json:"description"`
	SKU              *string `json:"sku"`
	Price            int64   `json:"price"`
	UnitCost         *int64  `json:"unitCost"`
	Unit             *string `json:"unit"`
	UnitOfMeasureID  *string `json:"unitOfMeasureId"`
	Type             string  `json:"type"`
	Category         *string `json:"category"`
	TaxRateID        *string `json:"taxRateId"`
	StockEnabled     int     `json:"stockEnabled"`
	Serialized       int     `json:"serialized"`
	RevenueAccountID *string `json:"revenueAccountId"`
	ExpenseAccountID *string `json:"expenseAccountId"`
	FamilyID         *string `json:"familyId"`
	// IsArchived is nil to keep the stored value: the drawer's full-replace
	// save and the Excel import (which has no such column) must never
	// unarchive a product by leaving it out.
	IsArchived *bool `json:"isArchived"`
}

// productCategories are the only values Product.Category may take besides
// nil ("unclassified") — the CHECK constraint in migration 0065 enforces the
// same set at the database level.
var productCategories = map[string]bool{
	"finished":  true,
	"component": true,
}

// ProductListOptions filters/pages/sorts GetProducts. Limit == 0 means "no
// limit" — the original full-fetch behavior every other caller (line-item
// product pickers on invoices/orders/deliveries/purchase-orders/
// inbound-deliveries) still relies on. SortField == "" keeps the original
// default order (name ascending).
type ProductListOptions struct {
	Search string
	// Type filters on the exact products.type value ("product"/"service")
	// when set; empty means no filter.
	Type string
	// Category filters on the exact products.category value ("finished"/
	// "component") when set; the special value "unclassified" matches
	// NULL — the app-level name productCategories/the frontend already use
	// for that state — since NULL can't be passed as a query param value.
	// Empty means no filter.
	Category string
	// FamilyID filters on products.familyId; the special value "none"
	// matches products with no family. Empty means no filter.
	FamilyID string
	// Stock filters stock-tracked products by quantity: "out" is nothing (or
	// less) left, "low" is LowStockThreshold (the Dashboard's low-stock rule,
	// db/dashboard_panels.go) or less, out of stock included.
	// Empty means no filter. With no sort field either one lists the lowest
	// stock first.
	Stock string
	// Archived: "exclude" leaves archived products out, "only" lists just
	// them; empty lists every product — what the pickers rely on to name an
	// archived product still on a saved document.
	Archived  string
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
	if opts.Type != "" {
		where += " AND p.type = ?"
		args = append(args, opts.Type)
	}
	if opts.Category == "unclassified" {
		where += " AND p.category IS NULL"
	} else if opts.Category != "" {
		where += " AND p.category = ?"
		args = append(args, opts.Category)
	}
	if opts.FamilyID == "none" {
		where += " AND p.familyId IS NULL"
	} else if opts.FamilyID != "" {
		where += " AND p.familyId = ?"
		args = append(args, opts.FamilyID)
	}
	switch opts.Archived {
	case "exclude":
		where += " AND p.isArchived = 0"
	case "only":
		where += " AND p.isArchived = 1"
	}
	switch opts.Stock {
	case "out":
		where += " AND p.stockEnabled = 1 AND p.stockQuantity <= 0"
	case "low":
		where += " AND p.stockEnabled = 1 AND p.stockQuantity <= ?"
		args = append(args, LowStockThreshold)
	}

	var total int
	if err := d.DB.Get(&total, "SELECT COUNT(*) "+from+" "+where, args...); err != nil {
		return nil, 0, fmt.Errorf("get_products_count: %w", err)
	}

	sortCol, ok := productSortColumns[opts.SortField]
	if !ok {
		sortCol = "p.name"
		if opts.Stock != "" {
			sortCol = "p.stockQuantity"
		}
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

// checkProductFKOwnership validates that taxRateId/revenueAccountId/
// expenseAccountId (if set) belong to the SAME organization as the product
// (issue #189). A nil/empty field means "not part of this request," not a
// mismatch.
// checkProductFamilyOwnership rejects a familyId from another organization
// (the issue #189 cross-org FK rule) or one that doesn't exist.
func (d *Database) checkProductFamilyOwnership(organizationID string, familyID *string) error {
	if familyID == nil || *familyID == "" {
		return nil
	}
	family, err := d.GetProductFamily(*familyID)
	if err != nil {
		return newValidationError("product family not found")
	}
	return requireSameOrg(organizationID, family.OrganizationID, "product family")
}

func (d *Database) checkProductFKOwnership(organizationID string, taxRateID, revenueAccountID, expenseAccountID *string) error {
	if taxRateID != nil && *taxRateID != "" {
		taxRate, err := d.GetTaxRate(*taxRateID)
		if err != nil {
			return newValidationError("tax rate not found")
		}
		if err := requireSameOrg(organizationID, taxRate.OrganizationID, "tax rate"); err != nil {
			return err
		}
	}
	if revenueAccountID != nil && *revenueAccountID != "" {
		account, err := d.GetAccount(*revenueAccountID)
		if err != nil {
			return newValidationError("revenue account not found")
		}
		if err := requireSameOrg(organizationID, account.OrganizationID, "revenue account"); err != nil {
			return err
		}
	}
	if expenseAccountID != nil && *expenseAccountID != "" {
		account, err := d.GetAccount(*expenseAccountID)
		if err != nil {
			return newValidationError("expense account not found")
		}
		if err := requireSameOrg(organizationID, account.OrganizationID, "expense account"); err != nil {
			return err
		}
	}
	return nil
}

// resolveProductUnit validates unitOfMeasureId belongs to the same
// organization (issue #189) and, when set, returns the selected unit of
// measure's name as the authoritative value for the legacy free-text Unit
// column — the same "denormalize once at the write boundary" shape BOM
// version lines already use for componentName/Sku/Unit (db/product_bom.go).
// This keeps every existing reader of products.unit (Inventory, Products
// list, BOM component display) correct with no changes of their own. A
// nil/empty unitOfMeasureId leaves fallbackUnit untouched, preserving plain
// free-text unit entry for any caller that doesn't use the structured list
// (cmd/seed-demo, a direct API client) — *unless* the product currently has
// one, in which case the stored Unit is a derived copy of that unit of
// measure's name and clearing the link must clear the copy too. Otherwise a
// caller that echoes the old Unit back while nulling unitOfMeasureId leaves
// a ghost value behind that nothing owns (F71). currentUnitOfMeasureID is
// nil on the create path, where there is no previous value to reason about.
func (d *Database) resolveProductUnit(
	organizationID string, unitOfMeasureID, fallbackUnit, currentUnitOfMeasureID *string,
) (*string, error) {
	if unitOfMeasureID == nil || *unitOfMeasureID == "" {
		if currentUnitOfMeasureID != nil && *currentUnitOfMeasureID != "" {
			return nil, nil
		}
		return fallbackUnit, nil
	}
	uom, err := d.GetUnitOfMeasure(*unitOfMeasureID)
	if err != nil {
		return nil, newValidationError("unit of measure not found")
	}
	if err := requireSameOrg(organizationID, uom.OrganizationID, "unit of measure"); err != nil {
		return nil, err
	}
	return &uom.Name, nil
}

// normalizeSKU uppercases and trims a product code so "air-filt-9180" and
// "AIR-FILT-9180" can't coexist as visually-different-but-logically-same
// values under the same (organizationId, sku) unique index — every SKU a
// user or the seed-demo tool ever types lands in one consistent case. A
// blank result (all whitespace, or already empty) normalizes to nil, the
// same "no SKU" convention every other optional product field uses, rather
// than storing a meaningless empty string.
func normalizeSKU(sku *string) *string {
	if sku == nil {
		return nil
	}
	trimmed := strings.ToUpper(strings.TrimSpace(*sku))
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (d *Database) CreateProduct(req CreateProductRequest) (*Product, error) {
	// F94: an empty-string optional FK id means "unset"; normalize it to
	// nil before the guard and the INSERT both see it (db/optional_id.go).
	req.TaxRateID = nilIfEmptyID(req.TaxRateID)
	req.RevenueAccountID = nilIfEmptyID(req.RevenueAccountID)
	req.ExpenseAccountID = nilIfEmptyID(req.ExpenseAccountID)
	req.UnitOfMeasureID = nilIfEmptyID(req.UnitOfMeasureID)
	req.FamilyID = nilIfEmptyID(req.FamilyID)

	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	if req.Type == "" {
		req.Type = "service"
	}
	req.SKU = normalizeSKU(req.SKU)
	if req.StockEnabled == 0 {
		req.Serialized = 0
	}
	// Category only ever means something for a physical product — a service
	// can't be "the component" or "the finished good" of anything.
	if req.Type != "product" {
		req.Category = nil
	}
	if req.Category != nil && !productCategories[*req.Category] {
		return nil, newValidationError("invalid product category %q", *req.Category)
	}
	if err := d.checkProductFKOwnership(req.OrganizationID, req.TaxRateID, req.RevenueAccountID, req.ExpenseAccountID); err != nil {
		return nil, err
	}
	if err := d.checkProductFamilyOwnership(req.OrganizationID, req.FamilyID); err != nil {
		return nil, err
	}
	unit, err := d.resolveProductUnit(req.OrganizationID, req.UnitOfMeasureID, req.Unit, nil)
	if err != nil {
		return nil, err
	}
	req.Unit = unit
	// A brand-new product always has zero stock, so the toggle guard
	// UpdateProduct enforces has nothing to check here.
	_, err = d.DB.Exec(
		`INSERT INTO products (id, organizationId, name, description, sku, price, unitCost, unit, unitOfMeasureId, type, category, taxRateId, stockEnabled, serialized, revenueAccountId, expenseAccountId, familyId)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Name, req.Description, req.SKU,
		req.Price, req.UnitCost, req.Unit, req.UnitOfMeasureID, req.Type, req.Category, req.TaxRateID, req.StockEnabled, req.Serialized,
		req.RevenueAccountID, req.ExpenseAccountID, req.FamilyID,
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
	// F94: an empty-string optional FK id means "unset"; normalize it to
	// nil before the guard and the INSERT both see it (db/optional_id.go).
	updates.TaxRateID = nilIfEmptyID(updates.TaxRateID)
	updates.RevenueAccountID = nilIfEmptyID(updates.RevenueAccountID)
	updates.ExpenseAccountID = nilIfEmptyID(updates.ExpenseAccountID)
	updates.UnitOfMeasureID = nilIfEmptyID(updates.UnitOfMeasureID)

	if updates.Type == "" {
		updates.Type = "service"
	}
	updates.SKU = normalizeSKU(updates.SKU)
	if updates.StockEnabled == 0 {
		updates.Serialized = 0
	}
	if updates.Type != "product" {
		updates.Category = nil
	}
	if updates.Category != nil && !productCategories[*updates.Category] {
		return nil, newValidationError("invalid product category %q", *updates.Category)
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
	if err := d.checkProductFKOwnership(current.OrganizationID, updates.TaxRateID, updates.RevenueAccountID, updates.ExpenseAccountID); err != nil {
		return nil, err
	}
	// familyId is three-state, unlike the rest of this full-replace update:
	// omitted (nil) keeps the stored family, "" clears it, an id sets it —
	// the organization-wide convention — so a caller that predates families
	// (an old client, a 15-column Excel sheet) can't wipe one it never saw.
	familyID := current.FamilyID
	if updates.FamilyID != nil {
		familyID = nilIfEmptyID(updates.FamilyID)
		if err := d.checkProductFamilyOwnership(current.OrganizationID, familyID); err != nil {
			return nil, err
		}
	}
	unit, err := d.resolveProductUnit(current.OrganizationID, updates.UnitOfMeasureID, updates.Unit, current.UnitOfMeasureID)
	if err != nil {
		return nil, err
	}
	updates.Unit = unit
	isArchived := current.IsArchived
	if updates.IsArchived != nil {
		isArchived = *updates.IsArchived
	}

	_, err = d.DB.Exec(
		`UPDATE products
		 SET name = ?, description = ?, sku = ?, price = ?, unitCost = ?, unit = ?, unitOfMeasureId = ?, type = ?, category = ?, taxRateId = ?,
		     stockEnabled = ?, serialized = ?, revenueAccountId = ?, expenseAccountId = ?, familyId = ?, isArchived = ?
		 WHERE id = ?`,
		updates.Name, updates.Description, updates.SKU, updates.Price,
		updates.UnitCost, updates.Unit, updates.UnitOfMeasureID, updates.Type, updates.Category, updates.TaxRateID, updates.StockEnabled, updates.Serialized,
		updates.RevenueAccountID, updates.ExpenseAccountID, familyID, isArchived,
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

// productReference is one table+column pair that points at products(id).
type productReference struct {
	Table  string
	Column string
	// Label names the rows in a refusal ("stock movements", …).
	Label string
}

// productReferencingTables lists every column that references a product.
// Deleting a product cascades its stock movements, serial numbers and
// recipes and nulls it out of every document line, while the GL entries
// those movements fed stay posted — the Inventory account would then carry
// value with nothing behind it (audit F167). So a product anything refers
// to can't be deleted, only archived. TestProductUsageCountCoversEveryReference
// reads the live schema's foreign keys and fails if one is missing here.
var productReferencingTables = []productReference{
	{"stockMovements", "productId", "stock movements"},
	{"product_serial_numbers", "productId", "serial numbers"},
	{"invoiceLineItems", "productId", "invoice lines"},
	{"incoming_invoice_line_items", "productId", "incoming invoice lines"},
	{"orderLineItems", "productId", "order lines"},
	{"purchase_order_line_items", "productId", "purchase order lines"},
	{"outbound_delivery_line_items", "productId", "delivery lines"},
	{"inbound_delivery_line_items", "productId", "goods receipt lines"},
	{"production_orders", "finishedProductId", "production orders"},
	{"production_order_component_lines", "componentProductId", "production order components"},
	{"bill_of_materials", "finishedProductId", "recipe lines"},
	{"bill_of_materials", "componentProductId", "recipes using it"},
	{"bill_of_materials_versions", "finishedProductId", "recipe versions"},
	{"bill_of_materials_version_lines", "componentProductId", "recipe versions using it"},
}

// ProductUsage is how many rows of one kind refer to a product.
type ProductUsage struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// productUsageWith runs on d.DB or an open transaction (DeleteProduct's).
func productUsageWith(q sqlGetExecer, productID string) ([]ProductUsage, int64, error) {
	usage := []ProductUsage{}
	var total int64
	for _, ref := range productReferencingTables {
		var n int64
		// Table/column names come from the package-level slice above, never from input.
		if err := q.Get(&n, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", ref.Table, ref.Column), productID); err != nil {
			return nil, 0, fmt.Errorf("product_usage %s: %w", ref.Table, err)
		}
		if n > 0 {
			usage = append(usage, ProductUsage{Label: ref.Label, Count: n})
			total += n
		}
	}
	return usage, total, nil
}

// GetProductUsage returns what refers to a product (only the kinds with any
// rows) and the total, so the product drawer can offer Delete only for an
// unused product and Archive otherwise.
func (d *Database) GetProductUsage(productID string) ([]ProductUsage, int64, error) {
	return productUsageWith(d.DB, productID)
}

// DeleteProduct deletes a product nothing refers to and refuses (409) one
// with history, naming what refers to it; such a product is archived
// instead. The check and the delete share one transaction.
func (d *Database) DeleteProduct(productID string) (bool, error) {
	tx, err := d.DB.Beginx()
	if err != nil {
		return false, fmt.Errorf("delete_product begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	usage, total, err := productUsageWith(tx, productID)
	if err != nil {
		return false, err
	}
	if total > 0 {
		parts := make([]string, len(usage))
		for i, u := range usage {
			parts[i] = fmt.Sprintf("%d %s", u.Count, u.Label)
		}
		return false, newValidationError(
			"this product can't be deleted because it has history (%s) — archive it instead",
			strings.Join(parts, ", "),
		)
	}
	res, err := tx.Exec(`DELETE FROM products WHERE id = ?`, productID)
	if err != nil {
		return false, fmt.Errorf("delete_product: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("delete_product commit: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
