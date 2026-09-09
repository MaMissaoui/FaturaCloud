package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestUpdateOrderStatus_RejectsIllegalTransition confirms
// db.orderStatusTransitions is enforced through PATCH .../status as a 409,
// not just reachable by calling UpdateOrderStatus directly in a db/ test —
// this pins the handler's writeMutationError wiring for the same status
// field UpdateOrderRequest deliberately has no field for (status is only
// ever settable through this route, never PUT).
func TestUpdateOrderStatus_RejectsIllegalTransition(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")
	orgID, _ := seedOrgAndClient(t, database)

	order, err := database.CreateOrder(db.CreateOrderRequest{
		ID: "order-1", OrganizationID: orgID, OrderNumber: "ORD-001", OrderDate: 1700000000000,
	})
	if err != nil {
		t.Fatalf("seed CreateOrder: %v", err)
	}
	if order.Status != "draft" {
		t.Fatalf("expected a fresh order to default to draft, got %q", order.Status)
	}

	// draft's only legal moves are confirmed/cancelled (db/order.go's
	// orderStatusTransitions) — jumping straight to delivered must be
	// rejected, not silently accepted or 500.
	body, _ := json.Marshal(map[string]string{"status": "delivered"})
	req := httptest.NewRequest(http.MethodPatch, "/api/orders/"+order.ID+"/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for draft->delivered, got %d: %s", rec.Code, rec.Body.String())
	}
}
