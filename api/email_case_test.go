package api

import (
	"net/http"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestEmailsAreCaseInsensitive pins audit F174: an email is stored
// normalized, signing in with other capitalization or stray spaces works,
// an account differing only by case is refused, and adding a member finds
// the account whatever the capitalization typed.
func TestEmailsAreCaseInsensitive(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "padmin", "admin", 1)
	admin := mintTestJWT(t, "padmin", "admin")

	rec := doJSON(t, mux, admin, http.MethodPost, "/api/users", map[string]any{
		"email": "  Probe.Case@Example.COM ", "password": "correct-horse-battery", "displayName": "Probe",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", rec.Code, rec.Body.String())
	}
	var stored string
	if err := database.DB.Get(&stored, `SELECT email FROM users WHERE displayName = 'Probe'`); err != nil {
		t.Fatal(err)
	}
	if stored != "probe.case@example.com" {
		t.Fatalf("stored email %q, want it normalized", stored)
	}

	for _, typed := range []string{"probe.case@example.com", "Probe.Case@Example.com", " PROBE.CASE@EXAMPLE.COM "} {
		rec := doJSON(t, mux, "", http.MethodPost, "/api/auth/login", map[string]any{
			"email": typed, "password": "correct-horse-battery",
		})
		if rec.Code != http.StatusOK {
			t.Errorf("sign in as %q: %d %s", typed, rec.Code, rec.Body.String())
		}
	}

	if rec := doJSON(t, mux, admin, http.MethodPost, "/api/users", map[string]any{
		"email": "PROBE.case@example.com", "password": "correct-horse-battery", "displayName": "Twin",
	}); rec.Code != http.StatusConflict {
		t.Fatalf("second account differing by case: %d, want 409", rec.Code)
	}

	org, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-email", Name: strPtr("Email Org")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.AddOrganizationUser(org.ID, "padmin", "admin"); err != nil {
		t.Fatal(err)
	}
	if rec := doJSON(t, mux, admin, http.MethodPost, "/api/organizations/"+org.ID+"/members", map[string]any{
		"email": "Probe.Case@Example.com", "role": "sales",
	}); rec.Code >= 300 {
		t.Fatalf("add member by other capitalization: %d %s", rec.Code, rec.Body.String())
	}
}
