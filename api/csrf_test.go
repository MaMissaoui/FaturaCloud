package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestCSRF_MutationRequiresHeader covers F26's CSRF defense: a state-changing
// request authenticated by the session cookie is rejected unless it also
// carries the custom CSRF header. This is the property the whole cookie
// migration rests on — without it, moving the token into a cookie would open a
// CSRF hole that the previous Authorization-header scheme didn't have.
func TestCSRF_MutationRequiresHeader(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "u", "user", 1)
	token := mintTestJWT(t, "u", "user")
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	body, _ := json.Marshal(map[string]any{"id": "c1", "organizationId": "org-1", "name": "X"})

	// Valid session cookie, but no CSRF header → 403.
	req := httptest.NewRequest(http.MethodPost, "/api/clients", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a cookie-auth mutation missing the CSRF header, got %d: %s", rec.Code, rec.Body.String())
	}

	// Same request with the CSRF header present → accepted (not 403).
	req2 := httptest.NewRequest(http.MethodPost, "/api/clients", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	authRequest(req2, token) // adds both the cookie and the CSRF header
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code == http.StatusForbidden {
		t.Fatalf("expected the CSRF header to satisfy the check, still got 403: %s", rec2.Body.String())
	}

	// A safe method (GET) is never gated by the CSRF check.
	req3 := httptest.NewRequest(http.MethodGet, "/api/organizations", nil)
	req3.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	rec3 := httptest.NewRecorder()
	mux.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected GET to bypass the CSRF check (200), got %d: %s", rec3.Code, rec3.Body.String())
	}
}
