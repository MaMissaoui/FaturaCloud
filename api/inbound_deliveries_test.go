package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestUpdateInboundDeliveryStatus_CancelledIsTerminal confirms
// db/inbound_delivery.go's inboundDeliveryStatusTransitions has no outgoing
// moves from "cancelled" — a receipt cancelled straight from draft (no
// stock/GRNI ever posted, so no reversal complications) must reject any
// further status change with a 409, at the HTTP layer.
func TestUpdateInboundDeliveryStatus_CancelledIsTerminal(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")
	orgID, _ := seedOrgAndClient(t, database)

	receipt, err := database.CreateInboundDelivery(db.CreateInboundDeliveryRequest{
		ID: "idl-1", OrganizationID: orgID, DeliveryNumber: "GR-001", DeliveryDate: 1700000000000,
	})
	if err != nil {
		t.Fatalf("seed CreateInboundDelivery: %v", err)
	}

	// draft -> cancelled is legal and, with no line items, touches no
	// stock/GL at all — isolates the transition-matrix check from every
	// other thing this status change can do.
	cancelBody, _ := json.Marshal(map[string]string{"status": "cancelled"})
	req := httptest.NewRequest(http.MethodPatch, "/api/inbound-deliveries/"+receipt.ID+"/status", bytes.NewReader(cancelBody))
	req.Header.Set("Content-Type", "application/json")
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for draft->cancelled, got %d: %s", rec.Code, rec.Body.String())
	}

	// cancelled is terminal — even falling back to draft must be rejected.
	reopenBody, _ := json.Marshal(map[string]string{"status": "draft"})
	req = httptest.NewRequest(http.MethodPatch, "/api/inbound-deliveries/"+receipt.ID+"/status", bytes.NewReader(reopenBody))
	req.Header.Set("Content-Type", "application/json")
	authRequest(req, token)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for cancelled->draft, got %d: %s", rec.Code, rec.Body.String())
	}
}
