package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
	"github.com/xuri/excelize/v2"
)

// TestUploadDocumentTemplate_UnknownOrganizationReturns404 covers the fix
// for an opaque 500: uploading a template for a nonexistent organizationId
// used to fail the INSERT's FK constraint and surface as a generic 500 —
// db.UploadDocumentTemplate now checks the organization exists first and
// returns sql.ErrNoRows, which the handler translates into a clean 404,
// matching the rest of this codebase's "409/404, never an opaque 500 from a
// FK" convention.
func TestUploadDocumentTemplate_UnknownOrganizationReturns404(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")

	body, contentType := multipartFile(t, "file", "template.xlsx", minimalXLSXBytes(t))
	req := httptest.NewRequest(http.MethodPost, "/api/organizations/does-not-exist/document-templates/invoice", body)
	req.Header.Set("Content-Type", contentType)
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a nonexistent organization, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestUploadDocumentTemplate_UnknownDocumentTypeReturns400 covers the fix
// for an unbounded/arbitrary documentType: an authenticated user could
// otherwise store a 2MB blob under any made-up key. db.IsKnownDocumentType
// is checked at the API layer before touching the database.
func TestUploadDocumentTemplate_UnknownDocumentTypeReturns400(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}

	body, contentType := multipartFile(t, "file", "template.xlsx", minimalXLSXBytes(t))
	req := httptest.NewRequest(http.MethodPost, "/api/organizations/org-1/document-templates/not-a-real-type", body)
	req.Header.Set("Content-Type", contentType)
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown document type, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestUploadAndListDocumentTemplates covers the round trip the Settings
// page's "Custom" vs "Default" tag relies on: before upload, the list is
// empty; after upload, it reports the override; after delete, it's empty
// again.
func TestUploadAndListDocumentTemplates(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}

	listReq := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/organizations/org-1/document-templates", nil)
		authRequest(req, token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	rec := listReq()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if bytes.TrimSpace(rec.Body.Bytes())[0] != '[' {
		t.Fatalf("expected a JSON array, got %s", rec.Body.String())
	}
	if !bytes.Equal(bytes.TrimSpace(rec.Body.Bytes()), []byte("[]")) {
		t.Fatalf("expected an empty list before any upload, got %s", rec.Body.String())
	}

	uploadBody, contentType := multipartFile(t, "file", "template.xlsx", minimalXLSXBytes(t))
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/organizations/org-1/document-templates/invoice", uploadBody)
	uploadReq.Header.Set("Content-Type", contentType)
	authRequest(uploadReq, token)
	uploadRec := httptest.NewRecorder()
	mux.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 from upload, got %d: %s", uploadRec.Code, uploadRec.Body.String())
	}

	rec = listReq()
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"documentType":"invoice"`)) {
		t.Fatalf("expected the list to report the invoice override, got %s", rec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/organizations/org-1/document-templates/invoice", nil)
	authRequest(deleteReq, token)
	deleteRec := httptest.NewRecorder()
	mux.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 from delete, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	rec = listReq()
	if !bytes.Equal(bytes.TrimSpace(rec.Body.Bytes()), []byte("[]")) {
		t.Fatalf("expected an empty list after delete, got %s", rec.Body.String())
	}
}

func multipartFile(t *testing.T, field, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, writer.FormDataContentType()
}

// minimalXLSXBytes returns a real, minimal, valid .xlsx file — a plain byte
// slice won't pass db.UploadDocumentTemplate's excelize.OpenReader check.
func minimalXLSXBytes(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture xlsx: %v", err)
	}
	return buf.Bytes()
}
