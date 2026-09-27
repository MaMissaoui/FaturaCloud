package db

import (
	"strings"
	"testing"
)

type stockCountFixture struct {
	orgID     string
	washer    *Product // SKU MAL-001, stock 5 costed at 100.00
	fridge    *Product // no SKU, stock 0
	service   *Product // not stock-tracked
	serial    *Product // serialized
	inventory string
}

func newStockCountFixture(t *testing.T, d *Database, orgID string) stockCountFixture {
	t.Helper()
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: orgID, Name: ptr("Stock Count Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	requireFiscalYearCoveringNow(t, d, org.ID)
	mk := func(req CreateProductRequest) *Product {
		t.Helper()
		req.OrganizationID = org.ID
		p, err := d.CreateProduct(req)
		if err != nil {
			t.Fatalf("CreateProduct %s: %v", req.Name, err)
		}
		return p
	}
	f := stockCountFixture{
		orgID:     org.ID,
		washer:    mk(CreateProductRequest{Name: "Machine à laver HGE 9kg", SKU: ptr("MAL-001"), Type: "product", StockEnabled: 1}),
		fridge:    mk(CreateProductRequest{Name: "Réfrigérateur Condor 360L blanc", Type: "product", StockEnabled: 1}),
		service:   mk(CreateProductRequest{Name: "Installation", Type: "service"}),
		serial:    mk(CreateProductRequest{Name: "TV 55\" TCL", SKU: ptr("TV-001"), Type: "product", StockEnabled: 1, Serialized: 1}),
		inventory: *org.DefaultInventoryAccountID,
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: f.washer.ID, Type: "in", Quantity: 5, UnitCost: ptr(int64(10000)),
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}
	return f
}

func stockCountRows(t *testing.T, d *Database, orgID string, rows [][]string) *MassDataImportResult {
	t.Helper()
	res, err := d.ImportStockCountXLSX(orgID, buildTestXLSX(t, stockMassDataSpec{}.Headers(), rows))
	if err != nil {
		t.Fatalf("ImportStockCountXLSX: %v", err)
	}
	return res
}

func productStock(t *testing.T, d *Database, id string) float64 {
	t.Helper()
	p, err := d.GetProduct(id)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	return p.StockQuantity
}

func TestStockCountExportListsCountableProductsWithBlankCount(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := newStockCountFixture(t, d, "org-stock-export")

	content, err := d.ExportStockCountXLSX(f.orgID)
	if err != nil {
		t.Fatalf("ExportStockCountXLSX: %v", err)
	}
	rows, err := readXLSXRows(t, content)
	if err != nil {
		t.Fatalf("readXLSXRows: %v", err)
	}
	if strings.Join(rows[0], "|") != strings.Join(stockMassDataSpec{}.Headers(), "|") {
		t.Fatalf("header = %v", rows[0])
	}
	got := map[string][]string{}
	for _, r := range rows[1:] {
		got[r[stockColName]] = r
	}
	if len(got) != 2 {
		t.Fatalf("exported %d products, want 2 (service and serialized excluded): %v", len(got), rows[1:])
	}
	washer := got[f.washer.Name]
	if washer[stockColID] != f.washer.ID || washer[stockColSKU] != "MAL-001" || washer[stockColCurrent] != "5" {
		t.Fatalf("washer row = %v", washer)
	}
	if len(washer) > stockColCounted && washer[stockColCounted] != "" {
		t.Fatalf("counted quantity should be exported blank, got %q", washer[stockColCounted])
	}

	// Uploading the untouched export changes nothing.
	res, err := d.ImportStockCountXLSX(f.orgID, content)
	if err != nil {
		t.Fatalf("ImportStockCountXLSX: %v", err)
	}
	if res.Unchanged != 2 || res.Updated != 0 || res.Failed != 0 {
		t.Fatalf("round trip = %+v, want 2 unchanged", res)
	}
}

// The count is the quantity on hand: the server posts the difference, and
// uploading the same file again moves nothing.
func TestStockCountSetsCountedQuantityAndIsIdempotent(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := newStockCountFixture(t, d, "org-stock-count")
	rows := [][]string{
		{f.washer.ID, "", "", "", "3", "", "Comptage"}, // by ID: 5 -> 3
		{"", "", "", "", "", "", ""},                   // blank row, ignored by the engine
		{"", f.fridge.Name, "", "", "4", "850", ""},    // by name: 0 -> 4, costed
	}

	res := stockCountRows(t, d, f.orgID, rows)
	if res.Updated != 2 || res.Failed != 0 {
		t.Fatalf("first upload = %+v, want 2 updated", res)
	}
	if got := productStock(t, d, f.washer.ID); got != 3 {
		t.Fatalf("washer stock = %v, want 3", got)
	}
	if got := productStock(t, d, f.fridge.ID); got != 4 {
		t.Fatalf("fridge stock = %v, want 4", got)
	}
	fridge, err := d.GetProduct(f.fridge.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if fridge.UnitCost == nil || *fridge.UnitCost != 85000 {
		t.Fatalf("fridge unitCost = %v, want 85000 from the Unit Cost column", fridge.UnitCost)
	}

	movements, err := d.GetProductStockMovements(f.washer.ID)
	if err != nil {
		t.Fatalf("GetProductStockMovements: %v", err)
	}
	var shortage *StockMovement
	for i := range movements {
		if movements[i].Type == "count_subtraction" {
			shortage = &movements[i]
		}
	}
	if shortage == nil || shortage.Quantity != -2 {
		t.Fatalf("expected one count_subtraction of -2, got %+v", movements)
	}
	if shortage.Reference == nil || !strings.HasPrefix(*shortage.Reference, "Stock upload ") {
		t.Fatalf("reference = %v, want \"Stock upload <date>\"", shortage.Reference)
	}
	if shortage.Note == nil || *shortage.Note != "Comptage" {
		t.Fatalf("note = %v, want Comptage", shortage.Note)
	}

	// Perpetual valuation: the costed addition posted Dr Inventory 3400.00.
	entries, err := d.GetJournalEntries(f.orgID, "", "")
	if err != nil {
		t.Fatalf("GetJournalEntries: %v", err)
	}
	var inventoryNet int64
	for _, e := range entries {
		lines, err := d.GetJournalEntryLines(e.ID)
		if err != nil {
			t.Fatalf("GetJournalEntryLines: %v", err)
		}
		dr, cr := sumLines(lines, f.inventory)
		inventoryNet += dr - cr
	}
	// +5 @ 100.00 seeded, -2 @ 100.00 shortage, +4 @ 850.00 counted in.
	if inventoryNet != 50000-20000+340000 {
		t.Fatalf("inventory GL net = %d, want %d", inventoryNet, 50000-20000+340000)
	}

	again := stockCountRows(t, d, f.orgID, rows)
	if again.Unchanged != 2 || again.Updated != 0 || again.Failed != 0 {
		t.Fatalf("second upload = %+v, want 2 unchanged", again)
	}
	if got := productStock(t, d, f.washer.ID); got != 3 {
		t.Fatalf("washer stock after re-upload = %v, want 3", got)
	}
}

func TestStockCountBlankCountSkipsAndZeroEmpties(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := newStockCountFixture(t, d, "org-stock-zero")

	res := stockCountRows(t, d, f.orgID, [][]string{{"", "", "MAL-001", "5", "", "", ""}})
	if res.Unchanged != 1 {
		t.Fatalf("blank count = %+v, want unchanged", res)
	}
	if got := productStock(t, d, f.washer.ID); got != 5 {
		t.Fatalf("stock after blank count = %v, want 5", got)
	}

	res = stockCountRows(t, d, f.orgID, [][]string{{"", "", "MAL-001", "", "0", "", ""}})
	if res.Updated != 1 {
		t.Fatalf("zero count = %+v, want updated", res)
	}
	if got := productStock(t, d, f.washer.ID); got != 0 {
		t.Fatalf("stock after zero count = %v, want 0", got)
	}
}

// A bad row is reported and skipped; the rest of the file still applies.
func TestStockCountReportsBadRowsAndAppliesTheRest(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := newStockCountFixture(t, d, "org-stock-errors")
	if _, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: f.orgID, Name: f.fridge.Name, SKU: ptr("REF-002"), Type: "product", StockEnabled: 1,
	}); err != nil {
		t.Fatalf("CreateProduct duplicate name: %v", err)
	}

	res := stockCountRows(t, d, f.orgID, [][]string{
		{"", "", "", "", "1", "", ""},               // no identification
		{"nope", "", "", "", "1", "", ""},           // unknown id
		{"", "", "NOPE-1", "", "1", "", ""},         // unknown SKU
		{"", "Unknown thing", "", "", "1", "", ""},  // unknown name
		{"", f.fridge.Name, "", "", "1", "", ""},    // ambiguous name
		{"", f.service.Name, "", "", "1", "", ""},   // not stock-tracked
		{"", "", "TV-001", "", "1", "", ""},         // serialized
		{"", "", "MAL-001", "", "-1", "", ""},       // negative
		{"", "", "MAL-001", "", "beaucoup", "", ""}, // not a number
		{"", "", "MAL-001", "", "7", "", ""},        // valid: 5 -> 7
	})
	if res.Failed != 9 || res.Updated != 1 {
		t.Fatalf("result = %+v, want 9 failed and 1 updated", res)
	}
	for _, r := range res.Rows[:9] {
		if r.Action != "error" || r.Error == "" {
			t.Fatalf("row %d = %+v, want an error", r.Row, r)
		}
	}
	if !strings.Contains(res.Rows[4].Error, "identify this row by Product ID or SKU") {
		t.Fatalf("ambiguous-name error = %q", res.Rows[4].Error)
	}
	if got := productStock(t, d, f.washer.ID); got != 7 {
		t.Fatalf("washer stock = %v, want 7", got)
	}
}

// The same product listed twice in one file (once by ID, once by SKU)
// ends at the last count, not at the sum of two deltas.
func TestStockCountSameProductTwiceUsesFreshStock(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := newStockCountFixture(t, d, "org-stock-twice")

	res := stockCountRows(t, d, f.orgID, [][]string{
		{f.washer.ID, "", "", "", "8", "", ""},
		{"", "", "MAL-001", "", "6", "", ""},
	})
	if res.Updated != 2 || res.Failed != 0 {
		t.Fatalf("result = %+v", res)
	}
	if got := productStock(t, d, f.washer.ID); got != 6 {
		t.Fatalf("washer stock = %v, want 6 (the last count)", got)
	}
}

// In perpetual valuation, a shortage on a product with no cost basis is
// refused by CreateStockMovement — reported on its own row, not the file.
func TestStockCountUncostedShortageIsARowError(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := newStockCountFixture(t, d, "org-stock-uncosted")

	res := stockCountRows(t, d, f.orgID, [][]string{
		{"", f.fridge.Name, "", "", "3", "", ""}, // uncosted in: fine, posts nothing
	})
	if res.Updated != 1 {
		t.Fatalf("uncosted addition = %+v, want updated", res)
	}
	res = stockCountRows(t, d, f.orgID, [][]string{
		{"", f.fridge.Name, "", "", "1", "", ""}, // shortage with no cost basis
	})
	if res.Failed != 1 || !strings.Contains(res.Rows[0].Error, "cost basis") {
		t.Fatalf("uncosted shortage = %+v, want a cost-basis row error", res)
	}
	if got := productStock(t, d, f.fridge.ID); got != 3 {
		t.Fatalf("fridge stock = %v, want 3 (unchanged)", got)
	}
}
