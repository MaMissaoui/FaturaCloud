package db

import (
	"errors"
	"strings"
	"testing"
)

// TestUpdateTaxRateRefusesAPercentageChangeOnDocuments covers audit F168:
// documents keep a rate's id, not its percentage, so changing the
// percentage of a rate already on documents rewrote their VAT in the Tax
// Summary, re-exports and e-invoices.
func TestUpdateTaxRateRefusesAPercentageChangeOnDocuments(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-1")
	fx.createInvoice(t, d, "inv-1", 2, 1000)

	seven := 7.0
	_, err := d.UpdateTaxRate(fx.taxRateID, UpdateTaxRateRequest{Percentage: &seven})
	var verr *ValidationError
	if !errors.As(err, &verr) || !strings.Contains(err.Error(), "create a new tax rate") {
		t.Fatalf("UpdateTaxRate(7%%) err = %v; want a *ValidationError pointing at a new rate", err)
	}
	rate, err := d.GetTaxRate(fx.taxRateID)
	if err != nil || rate.Percentage != 20 {
		t.Fatalf("rate after refusal = %+v, %v; want 20%% kept", rate, err)
	}

	// The form resends the unchanged percentage with every save.
	twenty, name := 20.0, "Standard VAT renamed"
	rate, err = d.UpdateTaxRate(fx.taxRateID, UpdateTaxRateRequest{Name: &name, Percentage: &twenty})
	if err != nil || rate.Name != name || rate.Percentage != 20 {
		t.Fatalf("save with an unchanged percentage = %+v, %v", rate, err)
	}

	lines, err := d.GetTaxRateDocumentLineCount(fx.taxRateID)
	if err != nil || lines != 1 {
		t.Fatalf("GetTaxRateDocumentLineCount = %d, %v; want 1", lines, err)
	}
}

// A rate that is only a product's default holds no VAT of its own, so its
// percentage may still change.
func TestUpdateTaxRateAllowsAPercentageChangeOffDocuments(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-1")
	if _, err := d.UpdateProduct(fx.productID, UpdateProductRequest{Name: "Widget", Price: 1000, TaxRateID: &fx.taxRateID}); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	nineteen := 19.0
	rate, err := d.UpdateTaxRate(fx.taxRateID, UpdateTaxRateRequest{Percentage: &nineteen})
	if err != nil || rate.Percentage != 19 {
		t.Fatalf("UpdateTaxRate(19%%) = %+v, %v; want allowed", rate, err)
	}
}

// Every line table that carries a taxRate column holds VAT computed from
// it, so it must be in taxRateDocumentLineTables or a rate on it could
// change its percentage.
func TestTaxRateDocumentLineTablesCoverEveryLineTable(t *testing.T) {
	listed := map[string]bool{}
	for _, table := range taxRateDocumentLineTables {
		listed[table] = true
	}
	for _, ref := range taxRateReferencingTables {
		if ref.Column == "taxRate" && !listed[ref.Table] {
			t.Errorf("%s.taxRate is a document line's rate but isn't in taxRateDocumentLineTables (db/tax_rate.go)", ref.Table)
		}
	}
}
