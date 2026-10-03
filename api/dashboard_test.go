package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// The Dashboard payload is shared by every member, except the till: it is
// Cash Book data, which the section guard keeps from sales, purchasing and
// accounting, so only a role that may use the Cash Book gets it.
func TestDashboardTillFollowsTheCashBookSection(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	org, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-dash", Name: strPtr("Dashboard Org")})
	if err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	roles := map[string]string{
		"dash-general": "general", "dash-sales": "sales",
		"dash-purchasing": "purchasing", "dash-accounting": "accounting",
	}
	for user, role := range roles {
		seedUser(t, database, user, "user", 1)
		if _, err := database.AddOrganizationUser(org.ID, user, role); err != nil {
			t.Fatalf("seed membership %s: %v", user, err)
		}
	}
	if _, err := database.CreateFiscalYear(db.CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2023-2099", StartDate: 1672531200000, EndDate: 4102444799000,
	}); err != nil {
		t.Fatalf("seed CreateFiscalYear: %v", err)
	}
	accounts, err := database.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	byCode := map[string]string{}
	for _, a := range accounts {
		byCode[a.Code] = a.ID
	}
	register, bank := byCode["1010"], byCode["1020"]
	if _, err := database.UpdateOrganization(org.ID, db.UpdateOrganizationRequest{DefaultCashRegisterAccountID: &register}); err != nil {
		t.Fatalf("seed register account: %v", err)
	}
	journals, err := database.GetJournals(org.ID)
	if err != nil {
		t.Fatalf("GetJournals: %v", err)
	}
	var cashJournal string
	for _, j := range journals {
		if j.Type == "cash" {
			cashJournal = j.ID
		}
	}
	entry, err := database.CreateJournalEntry(db.CreateJournalEntryRequest{
		OrganizationID: org.ID, JournalID: cashJournal, Date: time.Now().UnixMilli(), Description: "till float",
		Lines: []db.CreateJournalLineRequest{{AccountID: register, Debit: 5000}, {AccountID: bank, Credit: 5000}},
	})
	if err != nil {
		t.Fatalf("seed CreateJournalEntry: %v", err)
	}
	if _, err := database.PostJournalEntry(entry.ID); err != nil {
		t.Fatalf("seed PostJournalEntry: %v", err)
	}

	for user, role := range roles {
		rec := doJSON(t, mux, mintTestJWT(t, user, ""), http.MethodGet, "/api/organizations/"+org.ID+"/dashboard", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: GET dashboard = %d: %s", role, rec.Code, rec.Body.String())
		}
		var data db.DashboardData
		if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
			t.Fatalf("%s: decode: %v", role, err)
		}
		wantTill := role == "general"
		if (data.CashRegister != nil) != wantTill {
			t.Fatalf("%s: cashRegister = %+v, want present=%v", role, data.CashRegister, wantTill)
		}
		if wantTill && data.CashRegister.Today.Closing != 5000 {
			t.Fatalf("%s: till closing = %d, want 5000", role, data.CashRegister.Today.Closing)
		}
	}
}
