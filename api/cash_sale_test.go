package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestCashbookRoleSettlesALoanLine drives the Cash Book's loan flow as a
// cashbook-only member through the real router: record a loan sale, then
// settle its lines one at a time. Before POST /api/cash-sales/{id}/payments
// existed, the only settlement path was GET /api/invoices/{id} +
// POST /api/payments + PATCH …/state, and all three 403 for this role (audit
// F140).
func TestCashbookRoleSettlesALoanLine(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "cashier", "user", 1)
	seedUser(t, database, "seller", "user", 1)
	seedUser(t, database, "outsider", "user", 1)

	org, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-cb", Name: strPtr("Counter Org")})
	if err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-other"}); err != nil {
		t.Fatalf("seed CreateOrganization(other): %v", err)
	}
	for user, role := range map[string]string{"cashier": "cashbook", "seller": "sales"} {
		if _, err := database.AddOrganizationUser(org.ID, user, role); err != nil {
			t.Fatalf("seed membership %s: %v", user, err)
		}
	}
	if _, err := database.AddOrganizationUser("org-other", "outsider", "cashbook"); err != nil {
		t.Fatalf("seed outsider membership: %v", err)
	}
	if _, err := database.CreateFiscalYear(db.CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("seed CreateFiscalYear: %v", err)
	}
	client, err := database.CreateClient(db.CreateClientRequest{OrganizationID: org.ID, Name: strPtr("Walk-in")})
	if err != nil {
		t.Fatalf("seed CreateClient: %v", err)
	}
	accounts, err := database.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	byCode := map[string]string{}
	for _, a := range accounts {
		byCode[a.Code] = a.ID
	}
	outputTax := byCode["2200"]
	taxRate, err := database.CreateTaxRate(db.CreateTaxRateRequest{
		OrganizationID: org.ID, Name: "VAT", Percentage: 20, OutputTaxAccountID: &outputTax,
	})
	if err != nil {
		t.Fatalf("seed CreateTaxRate: %v", err)
	}
	register := byCode["1010"]
	if _, err := database.UpdateOrganization(org.ID, db.UpdateOrganizationRequest{DefaultCashRegisterAccountID: &register}); err != nil {
		t.Fatalf("seed register account: %v", err)
	}

	cashier := mintTestJWT(t, "cashier", "")
	rec := doJSON(t, mux, cashier, http.MethodPost, "/api/cash-sales", map[string]any{
		"organizationId": org.ID, "clientId": client.ID, "date": int64(1738368000000), "currency": "EUR",
		"lineItems": []map[string]any{
			{"quantity": 2, "unitPrice": 1000, "taxRate": taxRate.ID},
			{"quantity": 1, "unitPrice": 500, "taxRate": taxRate.ID},
		},
		"subTotal": 2500, "taxTotal": 500, "total": 3000, "amountReceived": 0,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("cashier POST /api/cash-sales: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var sale db.CashSaleResult
	if err := json.Unmarshal(rec.Body.Bytes(), &sale); err != nil {
		t.Fatalf("decode cash sale: %v", err)
	}
	items, err := database.GetInvoiceLineItems(sale.Invoice.ID)
	if err != nil {
		t.Fatalf("GetInvoiceLineItems: %v", err)
	}
	lineAmount := map[string]int64{}
	for _, item := range items {
		// 2 × 10.00 → 24.00 of the 30.00 total, 1 × 5.00 → 6.00.
		if item.UnitPrice == 1000 {
			lineAmount[item.ID] = 2400
		} else {
			lineAmount[item.ID] = 600
		}
	}
	path := "/api/cash-sales/" + sale.Invoice.ID + "/payments"

	// Neither a sales member nor a cashier of another organization may use it.
	for token, who := range map[string]string{mintTestJWT(t, "seller", ""): "sales", mintTestJWT(t, "outsider", ""): "other-org cashbook"} {
		rec = doJSON(t, mux, token, http.MethodPost, path, map[string]any{"invoiceLineItemId": items[0].ID, "amount": 100})
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
			t.Fatalf("%s POST %s: expected 403/404, got %d: %s", who, path, rec.Code, rec.Body.String())
		}
	}

	for i, item := range items {
		rec = doJSON(t, mux, cashier, http.MethodPost, path, map[string]any{
			"invoiceLineItemId": item.ID, "amount": lineAmount[item.ID], "date": int64(1738368000000),
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("cashier settle line %d: expected 201, got %d: %s", i+1, rec.Code, rec.Body.String())
		}
		var result db.CashSalePaymentResult
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode payment result: %v", err)
		}
		wantState := "sent"
		if i == len(items)-1 {
			wantState = "paid"
		}
		if result.Invoice.State != wantState {
			t.Fatalf("after settling line %d: invoice state %q, want %q", i+1, result.Invoice.State, wantState)
		}
	}
}
