package db

import (
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// buildTestXLSX assembles a minimal workbook (a header row plus rows) the
// same shape exportMassDataXLSX produces, for feeding into
// importMassDataXLSX in these tests without going through a real export
// first.
func buildTestXLSX(t *testing.T, headers []string, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck
	sheet := f.GetSheetName(0)
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			t.Fatalf("SetCellValue header: %v", err)
		}
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				t.Fatalf("SetCellValue row: %v", err)
			}
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer: %v", err)
	}
	return buf.Bytes()
}

func TestMassDataClientsExportImportRoundTrip(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-md-clients"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.CreateClient(CreateClientRequest{
		OrganizationID: org.ID, Name: ptr("Acme Ltd"), City: ptr("Berlin"),
	}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	spec := clientsMassDataSpec{}
	xlsxBytes, err := exportMassDataXLSX(spec, d, org.ID)
	if err != nil {
		t.Fatalf("exportMassDataXLSX: %v", err)
	}

	// Re-import the exported file unmodified: the existing client updates
	// (by id), and appending one new blank-id row creates a second one.
	rows, err := readXLSXRows(t, xlsxBytes)
	if err != nil {
		t.Fatalf("read exported rows: %v", err)
	}
	if len(rows) != 2 { // header + 1 client
		t.Fatalf("expected 1 exported client row, got %d data rows", len(rows)-1)
	}
	newRow := make([]string, len(rows[0]))
	copy(newRow, rows[1])
	newRow[0] = "" // blank id -> create
	newRow[1] = "Beta Corp"
	xlsxWithNewRow := buildTestXLSX(t, rows[0], [][]string{rows[1], newRow})

	result, err := importMassDataXLSX(spec, d, org.ID, xlsxWithNewRow)
	if err != nil {
		t.Fatalf("importMassDataXLSX: %v", err)
	}
	if result.Failed != 0 {
		t.Fatalf("expected no failures, got %d: %+v", result.Failed, result.Rows)
	}
	if result.Updated != 1 || result.Created != 1 {
		t.Fatalf("expected 1 updated + 1 created, got updated=%d created=%d", result.Updated, result.Created)
	}

	clients, err := d.GetClients(org.ID)
	if err != nil {
		t.Fatalf("GetClients: %v", err)
	}
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients after import, got %d", len(clients))
	}
	names := map[string]bool{}
	for _, c := range clients {
		if c.Name != nil {
			names[*c.Name] = true
		}
	}
	if !names["Acme Ltd"] || !names["Beta Corp"] {
		t.Fatalf("expected both Acme Ltd and Beta Corp present, got %v", names)
	}
}

func TestMassDataImportRejectsMissingRequiredField(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-md-clients-bad"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	spec := clientsMassDataSpec{}
	headers := spec.Headers()
	blankName := make([]string, len(headers))
	blankName[2] = "some-code" // non-blank so the row isn't skipped as entirely empty, but Name (index 1) stays blank
	xlsxBytes := buildTestXLSX(t, headers, [][]string{blankName})

	result, err := importMassDataXLSX(spec, d, org.ID, xlsxBytes)
	if err != nil {
		t.Fatalf("importMassDataXLSX: %v", err)
	}
	if result.Failed != 1 || result.Created != 0 {
		t.Fatalf("expected exactly 1 failed row, got created=%d updated=%d failed=%d", result.Created, result.Updated, result.Failed)
	}
	if !strings.Contains(result.Rows[0].Error, "name is required") {
		t.Fatalf("expected a 'name is required' error, got %q", result.Rows[0].Error)
	}
}

func TestMassDataImportRejectsBadFile(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-md-badfile"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := importMassDataXLSX(clientsMassDataSpec{}, d, org.ID, []byte("not an xlsx file")); err == nil {
		t.Fatal("expected an error for a non-xlsx upload")
	}
}

// TestMassDataAccountsImportForwardParentReference exercises the one piece
// of real cross-row state accountsMassDataSpec carries: a later row in the
// same file can name an earlier row's own Code as its parent, even though
// that account didn't exist before the import started.
func TestMassDataAccountsImportForwardParentReference(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-md-accounts"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	spec := accountsMassDataSpec{}
	headers := spec.Headers() // ID, Code, Name, Type, Group Account, Active, Parent Account Code, DATEV Account Number, Description
	parentRow := []string{"", "9000", "New Assets Group", "asset", "Yes", "Yes", "", "", ""}
	childRow := []string{"", "9010", "New Cash", "asset", "No", "Yes", "9000", "", ""}
	xlsxBytes := buildTestXLSX(t, headers, [][]string{parentRow, childRow})

	result, err := importMassDataXLSX(spec, d, org.ID, xlsxBytes)
	if err != nil {
		t.Fatalf("importMassDataXLSX: %v", err)
	}
	if result.Failed != 0 {
		t.Fatalf("expected no failures, got %+v", result.Rows)
	}
	if result.Created != 2 {
		t.Fatalf("expected 2 created accounts, got %d", result.Created)
	}

	accounts, err := d.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	var child *Account
	var parent *Account
	for i := range accounts {
		switch accounts[i].Code {
		case "9010":
			child = &accounts[i]
		case "9000":
			parent = &accounts[i]
		}
	}
	if child == nil || parent == nil {
		t.Fatalf("expected both accounts to exist, got %+v", accounts)
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Fatalf("expected child's parent to resolve to the parent created earlier in the same file, got %+v", child.ParentID)
	}
}

// TestMassDataProductsImportResolvesReferencesAndReportsBadOnes checks that
// products' Tax Rate/Unit of Measure/Account columns resolve by name/code,
// and that one row with an unresolvable reference fails without blocking
// the other, valid rows in the same file.
func TestMassDataProductsImportResolvesReferencesAndReportsBadOnes(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-md-products"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	taxRate, err := d.CreateTaxRate(CreateTaxRateRequest{OrganizationID: org.ID, Name: "Standard", Percentage: 19})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}
	unit, err := d.CreateUnitOfMeasure(CreateUnitOfMeasureRequest{OrganizationID: org.ID, Name: "Piece"})
	if err != nil {
		t.Fatalf("CreateUnitOfMeasure: %v", err)
	}
	_ = taxRate
	_ = unit

	spec := productsMassDataSpec{}
	headers := spec.Headers()
	// ID, Name, Description, SKU, Type, Category, Price, Unit Cost, Unit (legacy text), Unit of Measure, Tax Rate, Stock Enabled, Serialized, Revenue Account Code, Expense Account Code
	goodRow := []string{"", "Widget", "", "WID-1", "product", "", "19.99", "10.00", "", "Piece", "Standard", "Yes", "No", "", ""}
	badRow := []string{"", "Gadget", "", "GAD-1", "product", "", "29.99", "", "", "", "Nonexistent Rate", "No", "No", "", ""}
	xlsxBytes := buildTestXLSX(t, headers, [][]string{goodRow, badRow})

	result, err := importMassDataXLSX(spec, d, org.ID, xlsxBytes)
	if err != nil {
		t.Fatalf("importMassDataXLSX: %v", err)
	}
	if result.Created != 1 || result.Failed != 1 {
		t.Fatalf("expected 1 created + 1 failed, got created=%d failed=%d rows=%+v", result.Created, result.Failed, result.Rows)
	}

	products, _, err := d.GetProducts(org.ID, ProductListOptions{})
	if err != nil {
		t.Fatalf("GetProducts: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("expected exactly 1 product to have been created, got %d", len(products))
	}
	p := products[0]
	if p.Price != 1999 {
		t.Fatalf("expected price 1999 cents, got %d", p.Price)
	}
	if p.TaxRateID == nil || *p.TaxRateID != taxRate.ID {
		got := "<nil>"
		if p.TaxRateID != nil {
			got = *p.TaxRateID
		}
		t.Fatalf("expected tax rate resolved by name to %q, got %q", taxRate.ID, got)
	}
	if p.UnitOfMeasureID == nil || *p.UnitOfMeasureID != unit.ID {
		got := "<nil>"
		if p.UnitOfMeasureID != nil {
			got = *p.UnitOfMeasureID
		}
		t.Fatalf("expected unit of measure resolved by name to %q, got %q", unit.ID, got)
	}
}

// readXLSXRows is a small test helper that mirrors importMassDataXLSX's own
// row-reading step, for tests that need to inspect an exported file's
// contents directly.
func readXLSXRows(t *testing.T, content []byte) ([][]string, error) {
	t.Helper()
	f, err := excelize.OpenReader(strings.NewReader(string(content)))
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck
	return f.GetRows(f.GetSheetName(0))
}
