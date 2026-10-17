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
