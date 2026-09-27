package db

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// stockMassDataSpec is the physical stock count upload (Inventory screen's
// Download/Upload Excel) — a massDataSpec like the master-data ones, but a
// row doesn't create or update a record: it states how many units were
// counted, and ImportRow posts the difference against current stock as one
// count_addition/count_subtraction movement through CreateStockMovement, so
// stock quantity, average cost and (perpetual valuation) the GL behave
// exactly like a manual count typed in on the Inventory screen.
//
// The count is the quantity on hand, never a delta: re-uploading the same
// file finds nothing left to move (every row "unchanged"), which makes the
// engine's continue-past-a-failed-row behaviour safe — fix the rows that
// errored and upload the whole file again. A blank count skips the row
// rather than zeroing it (a forgotten row must never wipe stock, the same
// reasoning as mass data's "no bulk delete"); only a literal 0 empties it.
// Current Quantity is export-only, for reference, and ignored on import.
type stockMassDataSpec struct{}

const (
	stockColID = iota
	stockColName
	stockColSKU
	stockColCurrent
	stockColCounted
	stockColUnitCost
	stockColNote
)

// stockQuantityEpsilon matches db.UpdateProduct's tolerance for comparing
// stockQuantity, itself a SUM over REAL movement quantities.
const stockQuantityEpsilon = 1e-9

func (stockMassDataSpec) Headers() []string {
	return []string{"Product ID", "Product Name", "SKU", "Current Quantity", "Counted Quantity", "Unit Cost", "Note"}
}

// countableProduct: only a stock-enabled, non-serialized product can take a
// count movement (CreateStockMovement restricts a serialized one to in/out
// with serial numbers).
func countableProduct(p Product) bool {
	return p.StockEnabled == 1 && p.Serialized == 0
}

func (stockMassDataSpec) ExportRows(d *Database, organizationID string) ([][]string, error) {
	products, _, err := d.GetProducts(organizationID, ProductListOptions{})
	if err != nil {
		return nil, err
	}
	rows := [][]string{}
	for _, p := range products {
		if !countableProduct(p) {
			continue
		}
		// Counted Quantity stays blank: an untouched row is skipped on
		// upload, so only what was actually counted moves.
		rows = append(rows, []string{
			p.ID, p.Name, strOrEmpty(p.SKU),
			strconv.FormatFloat(p.StockQuantity, 'f', -1, 64), "", "", "",
		})
	}
	return rows, nil
}

type stockImportContext struct {
	byID      map[string]Product
	bySKU     map[string]string
	byName    map[string][]string
	reference string
}

func (stockMassDataSpec) NewImportContext(d *Database, organizationID string) (any, error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return nil, err
	}
	products, _, err := d.GetProducts(organizationID, ProductListOptions{})
	if err != nil {
		return nil, err
	}
	ctx := &stockImportContext{
		byID:   make(map[string]Product, len(products)),
		bySKU:  make(map[string]string, len(products)),
		byName: make(map[string][]string, len(products)),
		// One reference for the whole upload, dated in the organization's
		// zone (the calendar-day invariant) — makes an upload's movements
		// findable together in the movement list.
		reference: "Stock upload " + time.Now().In(orgLocation(org.Timezone)).Format("2006-01-02"),
	}
	for _, p := range products {
		ctx.byID[p.ID] = p
		if p.SKU != nil && *p.SKU != "" {
			ctx.bySKU[*p.SKU] = p.ID
		}
		ctx.byName[p.Name] = append(ctx.byName[p.Name], p.ID)
	}
	return ctx, nil
}

// resolveProduct finds the row's product by ID, then SKU, then exact name —
// so a template downloaded from the app (ID always filled) and a list typed
// from scratch with no IDs or SKUs both work. Exact, case-sensitive
// matching, the same stance as every other mass-data lookup.
func (ctx *stockImportContext) resolveProduct(id, sku, name string) (string, error) {
	switch {
	case id != "":
		if _, ok := ctx.byID[id]; !ok {
			return "", newValidationError("product id %q not found in this organization", id)
		}
		return id, nil
	case sku != "":
		productID, ok := ctx.bySKU[sku]
		if !ok {
			return "", newValidationError("no product with SKU %q", sku)
		}
		return productID, nil
	case name != "":
		ids := ctx.byName[name]
		switch len(ids) {
		case 0:
			return "", newValidationError("no product named %q", name)
		case 1:
			return ids[0], nil
		default:
			return "", newValidationError("%d products are named %q — identify this row by Product ID or SKU", len(ids), name)
		}
	}
	return "", newValidationError("product ID, SKU or name is required")
}

func (stockMassDataSpec) ImportRow(d *Database, organizationID string, ctxAny any, cells []string) (action, identifier string, err error) {
	ctx := ctxAny.(*stockImportContext)
	id := strings.TrimSpace(cells[stockColID])
	sku := strings.TrimSpace(cells[stockColSKU])
	name := strings.TrimSpace(cells[stockColName])
	identifier = name
	if identifier == "" {
		identifier = sku
	}

	productID, err := ctx.resolveProduct(id, sku, name)
	if err != nil {
		return "", identifier, err
	}
	identifier = ctx.byID[productID].Name

	countedCell := strings.TrimSpace(cells[stockColCounted])
	if countedCell == "" {
		return "unchanged", identifier, nil
	}
	counted, err := cellFloat(countedCell)
	if err != nil {
		return "", identifier, newValidationError("counted quantity: %v", err)
	}
	if counted < 0 || math.IsNaN(counted) || math.IsInf(counted, 0) {
		return "", identifier, newValidationError("counted quantity must be zero or more")
	}

	// Read fresh, not from the context: an earlier row in this same file
	// may already have moved this product (listed twice, by ID and by name).
	product, err := d.GetProduct(productID)
	if err != nil {
		return "", identifier, err
	}
	if product.StockEnabled != 1 {
		return "", identifier, newValidationError("%q is not stock-tracked", product.Name)
	}
	if product.Serialized == 1 {
		return "", identifier, newValidationError(
			"%q is serialized — record its stock with a Stock In/Out and serial numbers instead", product.Name,
		)
	}

	delta := counted - product.StockQuantity
	if math.Abs(delta) < stockQuantityEpsilon {
		return "unchanged", identifier, nil
	}

	req := CreateStockMovementRequest{
		OrganizationID: organizationID,
		ProductID:      productID,
		Quantity:       delta,
		Note:           cellStr(cells[stockColNote]),
		Reference:      &ctx.reference,
	}
	if delta > 0 {
		req.Type = "count_addition"
		// A cost only means something on stock coming in; on a shortage
		// the units leave at the running average, like any outflow.
		unitCost, err := cellToCentsPtr(cells[stockColUnitCost])
		if err != nil {
			return "", identifier, newValidationError("unit cost: %v", err)
		}
		req.UnitCost = unitCost
	} else {
		req.Type = "count_subtraction"
	}
	if _, err := d.CreateStockMovement(req); err != nil {
		return "", identifier, err
	}
	return "updated", identifier, nil
}

// ExportStockCountXLSX/ImportStockCountXLSX are the public entry points for
// the Inventory screen's stock count download/upload (api/mass_data.go).
func (d *Database) ExportStockCountXLSX(organizationID string) ([]byte, error) {
	return exportMassDataXLSX(stockMassDataSpec{}, d, organizationID)
}

func (d *Database) ImportStockCountXLSX(organizationID string, content []byte) (*MassDataImportResult, error) {
	return importMassDataXLSX(stockMassDataSpec{}, d, organizationID, content)
}
