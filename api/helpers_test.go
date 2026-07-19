package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestDecodeJSON_BodyTooLarge covers F19: an oversized JSON body is rejected
// with 413 instead of being decoded unbounded into memory.
func TestDecodeJSON_BodyTooLarge(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")

	// A JSON document comfortably larger than maxJSONBody (10 MiB).
	huge := `{"name":"` + strings.Repeat("A", 11<<20) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/clients", strings.NewReader(huge))
	authRequest(req, token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for an oversized body, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDecodeJSON_NormalBodyAccepted is the counterpart — a modest,
// well-formed body (larger than a trivial payload, on the order of a real
// logo) is decoded normally and not tripped by the cap.
func TestDecodeJSON_NormalBodyAccepted(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed org: %v", err)
	}

	// ~1 MiB of notes — well under the 10 MiB cap.
	name := "ACME"
	big := strings.Repeat("x", 1<<20)
	payload := map[string]any{
		"id":             "client-1",
		"organizationId": "org-1",
		"name":           name,
		"address":        big,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/clients", bytes.NewReader(body))
	authRequest(req, token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for a valid ~1MiB body, got %d: %s", rec.Code, rec.Body.String())
	}
}
