package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestUpdateDeliveryStatus_CancelledIsTerminal mirrors
// TestUpdateInboundDeliveryStatus_CancelledIsTerminal for outbound
// deliveries — db/delivery.go's deliveryStatusTransitions has no
// "cancelled" key at all, so there is no legal move out of it. A draft
// delivery with no line items cancels cleanly (no stock ever moved) and
// then must reject falling back to draft.
func TestUpdateDeliveryStatus_CancelledIsTerminal(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")
	orgID, _ := seedOrgAndClient(t, database)

	delivery, err := database.CreateDelivery(db.CreateDeliveryRequest{
		ID: "od-1", OrganizationID: orgID, DeliveryNumber: "DEL-001", DeliveryDate: 1700000000000,
	})
	if err != nil {
		t.Fatalf("seed CreateDelivery: %v", err)
	}

	cancelBody, _ := json.Marshal(map[string]string{"status": "cancelled"})
	req := httptest.NewRequest(http.MethodPatch, "/api/deliveries/"+delivery.ID+"/status", bytes.NewReader(cancelBody))
	req.Header.Set("Content-Type", "application/json")
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for draft->cancelled, got %d: %s", rec.Code, rec.Body.String())
	}

	reopenBody, _ := json.Marshal(map[string]string{"status": "draft"})
	req = httptest.NewRequest(http.MethodPatch, "/api/deliveries/"+delivery.ID+"/status", bytes.NewReader(reopenBody))
	req.Header.Set("Content-Type", "application/json")
	authRequest(req, token)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for cancelled->draft, got %d: %s", rec.Code, rec.Body.String())
	}
}
