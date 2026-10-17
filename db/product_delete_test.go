package db

import (
	"errors"
	"strings"
	"testing"
)

// TestProductUsageCountCoversEveryReference reads every foreign key in the
// live schema and fails if one pointing at products isn't counted by
// productReferencingTables — DeleteProduct's guard would not see its rows,
// and a delete would cascade or null them away (audit F167).
func TestProductUsageCountCoversEveryReference(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	tables := []string{}
	if err := d.DB.Select(&tables,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`,
	); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	covered := map[string]bool{}
	for _, ref := range productReferencingTables {
		covered[ref.Table+"."+ref.Column] = true
	}
	seen := map[string]bool{}
	for _, table := range tables {
		fks := []struct {
			Table string `db:"table"`
			From  string `db:"from"`
		}{}
		if err := d.DB.Select(&fks, `SELECT "table", "from" FROM pragma_foreign_key_list(?)`, table); err != nil {
			t.Fatalf("pragma_foreign_key_list(%s): %v", table, err)
		}
		for _, fk := range fks {
			if fk.Table != "products" {
				continue
			}
			key := table + "." + fk.From
			seen[key] = true
			if !covered[key] {
				t.Errorf("%s references products but is not in productReferencingTables — "+
					"DeleteProduct would cascade or null it away; add it in db/product.go", key)
			}
		}
	}
	for key := range covered {
		if !seen[key] {
			t.Errorf("productReferencingTables lists %s, which has no foreign key to products", key)
		}
	}
}

func TestDeleteProductRefusesAProductWithHistory(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	product := seedCostProduct(t, d, "org-1")
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: "org-1", ProductID: product.ID, Type: "in", Quantity: 3,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}

	ok, err := d.DeleteProduct(product.ID)
	if ok || err == nil {
		t.Fatalf("DeleteProduct = %v, %v; want a refusal", ok, err)
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v; want a *ValidationError (409)", err)
	}
	if !strings.Contains(err.Error(), "1 stock movements") || !strings.Contains(err.Error(), "archive") {
		t.Fatalf("err = %q; want it to name the stock movement and point at archiving", err)
	}
	if _, err := d.GetProduct(product.ID); err != nil {
		t.Fatalf("product gone after a refused delete: %v", err)
	}
	var movements int
	if err := d.DB.Get(&movements, `SELECT COUNT(*) FROM stockMovements WHERE productId = ?`, product.ID); err != nil || movements != 1 {
		t.Fatalf("stock movements = %d, %v; want 1 kept", movements, err)
	}

	usage, total, err := d.GetProductUsage(product.ID)
	if err != nil || total != 1 || len(usage) != 1 || usage[0].Label != "stock movements" {
		t.Fatalf("GetProductUsage = %+v, %d, %v; want one stock movement", usage, total, err)
	}
}

func TestDeleteProductDeletesAnUnusedProduct(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	product := seedCostProduct(t, d, "org-1")

	usage, total, err := d.GetProductUsage(product.ID)
	if err != nil || total != 0 || len(usage) != 0 {
		t.Fatalf("GetProductUsage = %+v, %d, %v; want nothing", usage, total, err)
	}
	ok, err := d.DeleteProduct(product.ID)
	if !ok || err != nil {
		t.Fatalf("DeleteProduct = %v, %v; want deleted", ok, err)
	}
	if _, err := d.GetProduct(product.ID); err == nil {
		t.Fatal("product still there after delete")
	}
}

// forceDeleteProduct deletes a product the way DeleteProduct did before
// audit F167's guard, cascading its rows. Production can still hold
// documents and recipe versions whose product was deleted that way, so the
// tests of what survives such a delete (denormalized names, SKUs, units)
// keep using it.
func forceDeleteProduct(t *testing.T, d *Database, productID string) {
	t.Helper()
	if _, err := d.DB.Exec(`DELETE FROM products WHERE id = ?`, productID); err != nil {
		t.Fatalf("delete product %s: %v", productID, err)
	}
}

func TestArchiveProductKeepsItUnlessAskedAndFiltersLists(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	active, err := d.CreateProduct(CreateProductRequest{OrganizationID: org.ID, Name: "Active", Type: "product", StockEnabled: 1})
	if err != nil {
		t.Fatalf("CreateProduct active: %v", err)
	}
	retired, err := d.CreateProduct(CreateProductRequest{OrganizationID: org.ID, Name: "Retired", Type: "product", StockEnabled: 1})
	if err != nil {
		t.Fatalf("CreateProduct retired: %v", err)
	}
	if active.IsArchived || retired.IsArchived {
		t.Fatal("a new product must not be archived")
	}

	yes, no := true, false
	got, err := d.UpdateProduct(retired.ID, UpdateProductRequest{Name: "Retired", Type: "product", StockEnabled: 1, IsArchived: &yes})
	if err != nil || !got.IsArchived {
		t.Fatalf("archive: %+v, %v", got, err)
	}
	// A full-replace save that leaves the field out (the drawer before it
	// knew the field, the Excel import) keeps the product archived.
	got, err = d.UpdateProduct(retired.ID, UpdateProductRequest{Name: "Retired renamed", Type: "product", StockEnabled: 1})
	if err != nil || !got.IsArchived || got.Name != "Retired renamed" {
		t.Fatalf("save without isArchived: %+v, %v; want still archived", got, err)
	}

	names := func(archived string) []string {
		t.Helper()
		products, total, err := d.GetProducts(org.ID, ProductListOptions{Archived: archived})
		if err != nil {
			t.Fatalf("GetProducts(%q): %v", archived, err)
		}
		out := []string{}
		for _, p := range products {
			out = append(out, p.Name)
		}
		if total != len(out) {
			t.Fatalf("GetProducts(%q) total = %d, rows = %d", archived, total, len(out))
		}
		return out
	}
	if all := names(""); len(all) != 2 {
		t.Fatalf("no filter = %v; want both (saved documents name archived products)", all)
	}
	if ex := names("exclude"); len(ex) != 1 || ex[0] != "Active" {
		t.Fatalf("exclude = %v; want [Active]", ex)
	}
	if only := names("only"); len(only) != 1 || only[0] != "Retired renamed" {
		t.Fatalf("only = %v; want [Retired renamed]", only)
	}

	// The chip counts leave the archived product out and count it apart;
	// both products are out of stock, but only the active one needs restocking.
	summaries, err := d.GetProductSummaries(org.ID)
	if err != nil {
		t.Fatalf("GetProductSummaries: %v", err)
	}
	if summaries.ItemCount != 1 || summaries.OutOfStock != 1 || summaries.LowStock != 1 || summaries.Archived != 1 {
		t.Fatalf("summaries = %+v; want 1 item, 1 out, 1 low, 1 archived", summaries)
	}
	low, err := d.getLowStock(org.ID)
	if err != nil || low.Count != 1 || len(low.Items) != 1 || low.Items[0].ProductID != active.ID {
		t.Fatalf("getLowStock = %+v, %v; want only the active product", low, err)
	}

	got, err = d.UpdateProduct(retired.ID, UpdateProductRequest{Name: "Retired renamed", Type: "product", StockEnabled: 1, IsArchived: &no})
	if err != nil || got.IsArchived {
		t.Fatalf("restore: %+v, %v", got, err)
	}
}
