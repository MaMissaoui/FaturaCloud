package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func loanRegisterUpload(t *testing.T, mux http.Handler, token, orgID string, dryRun bool) *httptest.ResponseRecorder {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck
	rows := [][]any{
		{"Réf.", "Date", "Client", "CIN", "Tél", "Tél 2", "Adresse", "Garant", "Article", "Qté", "Montant", "Payé", "Dernier", "Note"},
		{"C1-P001-1", "15/06/2024", "Nouveau Client", "01234567", "", "", "", "", "Frigo", 1, 1200, 200, "", ""},
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			_ = f.SetCellValue("Sheet1", cell, v)
		}
	}
	var sheet bytes.Buffer
	if err := f.Write(&sheet); err != nil {
		t.Fatalf("write sheet: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "registre.xlsx")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	_, _ = part.Write(sheet.Bytes())
	_ = writer.WriteField("cutoverDate", "1738411200000") // calendarDayMs(2025-02-01)
	if !dryRun {
		_ = writer.WriteField("dryRun", "false")
	}
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/organizations/"+orgID+"/loan-imports", body)
	authRequest(req, token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// The loan register import is org-admin only, end to end through the real
// router: a dry run writes nothing, the import writes one batch, a general
// member is refused, and another organization's admin can't undo it.
func TestLoanRegisterImportThroughTheRouter(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	for _, u := range []string{"owner", "clerk", "outsider"} {
		seedUser(t, database, u, "user", 1)
	}
	org, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-loans", Name: strPtr("Loans Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-other"}); err != nil {
		t.Fatalf("CreateOrganization(other): %v", err)
	}
	for user, role := range map[string]string{"owner": "admin", "clerk": "general"} {
		if _, err := database.AddOrganizationUser(org.ID, user, role); err != nil {
			t.Fatalf("membership %s: %v", user, err)
		}
	}
	if _, err := database.AddOrganizationUser("org-other", "outsider", "admin"); err != nil {
		t.Fatalf("outsider membership: %v", err)
	}
	if _, err := database.CreateFiscalYear(db.CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
	owner, clerk, outsider := mintTestJWT(t, "owner", ""), mintTestJWT(t, "clerk", ""), mintTestJWT(t, "outsider", "")

	if rec := loanRegisterUpload(t, mux, clerk, org.ID, true); rec.Code != http.StatusForbidden {
		t.Fatalf("general member dry run: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	rec := doJSON(t, mux, clerk, http.MethodGet, "/api/organizations/"+org.ID+"/loan-imports/template", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("general member template: expected 403, got %d", rec.Code)
	}

	rec = loanRegisterUpload(t, mux, owner, org.ID, true)
	var report db.LoanImportReport
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &report) != nil || !report.DryRun || report.Imported || report.Loans != 1 {
		t.Fatalf("dry run: %d %s", rec.Code, rec.Body.String())
	}
	if want := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC).UnixMilli(); report.CutoverDate != want {
		t.Fatalf("cutover = %d, want 1 February 2025 (UTC, no zone set) %d", report.CutoverDate, want)
	}

	rec = loanRegisterUpload(t, mux, owner, org.ID, false)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &report) != nil || !report.Imported || report.BatchID == "" {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, mux, owner, http.MethodGet, "/api/organizations/"+org.ID+"/loan-imports", nil)
	var batches []db.LoanImportBatch
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &batches) != nil || len(batches) != 1 || batches[0].LoanCount != 1 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	if batches[0].CreatedBy == nil || *batches[0].CreatedBy != "owner" || batches[0].FileName == nil || *batches[0].FileName != "registre.xlsx" {
		t.Fatalf("batch = %+v, want created by owner from registre.xlsx", batches[0])
	}

	if rec := doJSON(t, mux, outsider, http.MethodDelete, "/api/loan-imports/"+report.BatchID, nil); rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("other org's admin undo: expected 403/404, got %d", rec.Code)
	}
	if rec := doJSON(t, mux, owner, http.MethodDelete, "/api/loan-imports/"+report.BatchID, nil); rec.Code != http.StatusOK {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body.String())
	}

	// The history has the import and its undo, named by the file; the dry
	// run changed nothing and isn't there.
	events, err := database.ListAuditEvents(db.AuditEventFilter{OrganizationID: org.ID})
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	methods := map[string]int{}
	for _, e := range events {
		if e.Resource != "loan-imports" {
			continue
		}
		methods[e.Method]++
		if e.EntityLabel != "registre.xlsx" {
			t.Errorf("%s %s named %q, want registre.xlsx", e.Method, e.Route, e.EntityLabel)
		}
	}
	if methods["POST"] != 1 || methods["DELETE"] != 1 {
		t.Errorf("loan import rows = %v, want one import (not the dry run) and one undo", methods)
	}
}
