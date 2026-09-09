package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestUpdatePurchaseOrderStatus_RejectsIllegalTransition mirrors
// TestUpdateOrderStatus_RejectsIllegalTransition for purchase orders'
// separate transition matrix (db/purchase_order.go's
// purchaseOrderStatusTransitions) — draft can only move to
// confirmed/cancelled, so a direct jump to received must 409 at the HTTP
// layer.
func TestUpdatePurchaseOrderStatus_RejectsIllegalTransition(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")
	orgID, _ := seedOrgAndClient(t, database)

	po, err := database.CreatePurchaseOrder(db.CreatePurchaseOrderRequest{
		ID: "po-1", OrganizationID: orgID, OrderNumber: "PO-001", OrderDate: 1700000000000,
	})
	if err != nil {
		t.Fatalf("seed CreatePurchaseOrder: %v", err)
	}
	if po.Status != "draft" {
		t.Fatalf("expected a fresh purchase order to default to draft, got %q", po.Status)
	}

	body, _ := json.Marshal(map[string]string{"status": "received"})
	req := httptest.NewRequest(http.MethodPatch, "/api/purchase-orders/"+po.ID+"/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for draft->received, got %d: %s", rec.Code, rec.Body.String())
	}
}
