package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestCreateJournalEntry_IgnoresSourceDocument is audit F191's regression
// test. A manual entry used to store a client-sent sourceDocumentType/Id,
// and an automatic posting skips any document that already has a live
// posted entry, so posting such an entry stopped the named invoice's own
// receivable and revenue from ever being posted. The fields are now
// ignored: the entry posts unclaimed and the invoice still posts.
func TestCreateJournalEntry_IgnoresSourceDocument(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "accountant", "user", 1)
	token := mintTestJWT(t, "accountant", "user")
	org, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-je-source"})
	if err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser(org.ID, "accountant", "accounting"); err != nil {
		t.Fatalf("seed org membership: %v", err)
	}
	const date = int64(1738368000000) // 2025-02-01
	if _, err := database.CreateFiscalYear(db.CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("seed CreateFiscalYear: %v", err)
	}
	journal, err := database.CreateJournal(db.CreateJournalRequest{
		OrganizationID: org.ID, Code: "MAN", Name: "Manual", Type: "miscellaneous",
	})
	if err != nil {
		t.Fatalf("seed CreateJournal: %v", err)
	}
	client, err := database.CreateClient(db.CreateClientRequest{OrganizationID: org.ID, Name: strPtr("Client")})
	if err != nil {
		t.Fatalf("seed CreateClient: %v", err)
	}
	invoice, err := database.CreateInvoice(db.CreateInvoiceRequest{
		OrganizationID: org.ID, Number: "INV-1", ClientID: client.ID, Date: date, Currency: "EUR",
		SubTotal: 1000, Total: 1000,
		LineItems: []db.CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 1000}},
	})
	if err != nil {
		t.Fatalf("seed CreateInvoice: %v", err)
	}

	send := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var reader *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			reader = bytes.NewReader(b)
		} else {
			reader = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
		authRequest(req, token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	rec := send(http.MethodPost, "/api/journal-entries", map[string]any{
		"organizationId": org.ID, "journalId": journal.ID, "date": date, "description": "claims INV-1",
		"sourceDocumentType": "invoice", "sourceDocumentId": invoice.ID,
		"lines": []map[string]any{
			{"accountId": *org.DefaultArAccountID, "debit": 1},
			{"accountId": *org.DefaultRevenueAccountID, "credit": 1},
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create journal entry: %d %s", rec.Code, rec.Body.String())
	}
	var entry db.JournalEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entry); err != nil {
		t.Fatalf("decode entry: %v", err)
	}
	if entry.SourceDocumentType != nil || entry.SourceDocumentID != nil {
		t.Fatalf("manual entry kept a source document: %v / %v", entry.SourceDocumentType, entry.SourceDocumentID)
	}
	if rec := send(http.MethodPatch, "/api/journal-entries/"+entry.ID+"/post", nil); rec.Code != http.StatusOK {
		t.Fatalf("post journal entry: %d %s", rec.Code, rec.Body.String())
	}

	if _, err := database.UpdateInvoiceState(invoice.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	posted, err := database.FindPostedEntryForSourceDocument(invoice.OrganizationID, "invoice", invoice.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if posted == nil || posted.ID == entry.ID {
		t.Fatalf("the invoice's own entry was not posted (found %+v)", posted)
	}
	lines, err := database.GetJournalEntryLines(posted.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	var arDebit, revenueCredit int64
	for _, l := range lines {
		if l.AccountID == *org.DefaultArAccountID {
			arDebit += l.Debit
		}
		if l.AccountID == *org.DefaultRevenueAccountID {
			revenueCredit += l.Credit
		}
	}
	if arDebit != 1000 || revenueCredit != 1000 {
		t.Fatalf("invoice entry AR debit %d, revenue credit %d; want 1000 each", arDebit, revenueCredit)
	}
}
