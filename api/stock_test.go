package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestCreateStockMovement_IgnoresClientID is audit F196's regression test at
// the API: a movement's id is also its batch's sourceDocumentId, and a Cash
// Book sale's stock is summed by that id, so a chosen id could attach a
// manual movement to an existing sale. The server now always picks the id.
func TestCreateStockMovement_IgnoresClientID(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "clerk", "user", 1)
	org, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-stock-id"})
	if err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser(org.ID, "clerk", "general"); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	product, err := database.CreateProduct(db.CreateProductRequest{
		OrganizationID: org.ID, Name: "Washer", Type: "product", StockEnabled: 1, Price: 1000,
	})
	if err != nil {
		t.Fatalf("seed CreateProduct: %v", err)
	}

	const chosen = "a-sale-id-chosen-here"
	rec := doJSON(t, mux, mintTestJWT(t, "clerk", ""), http.MethodPost, "/api/stock-movements", map[string]any{
		"id": chosen, "organizationId": org.ID, "productId": product.ID, "type": "in", "quantity": 3,
	})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create movement: %d %s", rec.Code, rec.Body.String())
	}
	var result db.CreateStockMovementResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Movements) != 1 {
		t.Fatalf("got %d movements, want 1", len(result.Movements))
	}
	m := result.Movements[0]
	if m.ID == chosen || (m.SourceDocumentID != nil && *m.SourceDocumentID == chosen) {
		t.Fatalf("the client's id was kept: id=%s sourceDocumentId=%v", m.ID, m.SourceDocumentID)
	}
}
