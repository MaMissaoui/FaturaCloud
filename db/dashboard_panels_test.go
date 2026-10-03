package db

import (
	"testing"
	"time"
)

// At UTC+14 the local day runs ahead of UTC: a "today" taken in UTC would
// read the till's yesterday as today.
func TestDashboardCashRegisterUsesTheOrganizationsDays(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-dashboard-till")
	register, bank, _ := cashMovementTestAccounts(t, d, fx.orgID)
	loc, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Skipf("zone data unavailable: %v", err)
	}
	if _, err := d.DB.Exec(`UPDATE organizations SET timezone = ?, defaultCashRegisterAccountId = ? WHERE id = ?`,
		loc.String(), register.ID, fx.orgID); err != nil {
		t.Fatalf("set timezone/register: %v", err)
	}

	yesterday := time.Date(2025, 2, 1, 10, 0, 0, 0, loc) // 2025-01-31 20:00 UTC
	today := time.Date(2025, 2, 2, 3, 0, 0, 0, loc)      // 2025-02-01 13:00 UTC
	now := time.Date(2025, 2, 2, 5, 0, 0, 0, loc)        // 2025-02-01 15:00 UTC
	postManualEntryForBalanceTest(t, d, fx.orgID, register.ID, bank.ID, 2000, yesterday.UnixMilli())
	postManualEntryForBalanceTest(t, d, fx.orgID, bank.ID, register.ID, 300, today.UnixMilli())

	till, err := d.getDashboardCashRegister(fx.orgID, now, loc)
	if err != nil {
		t.Fatalf("getDashboardCashRegister: %v", err)
	}
	if till == nil {
		t.Fatal("expected a till")
	}
	if till.AccountID != register.ID {
		t.Fatalf("account = %s, want %s", till.AccountID, register.ID)
	}
	if y := till.Yesterday; y.Date != "2025-02-01" || y.Opening != 0 || y.In != 2000 || y.Out != 0 || y.Closing != 2000 {
		t.Fatalf("yesterday = %+v, want 2025-02-01 0/+2000/-0/2000", y)
	}
	if td := till.Today; td.Date != "2025-02-02" || td.Opening != 2000 || td.In != 0 || td.Out != 300 || td.Closing != 1700 {
		t.Fatalf("today = %+v, want 2025-02-02 2000/+0/-300/1700", td)
	}
}

// No till to show is not an error: the Dashboard must still load.
func TestDashboardCashRegisterAbsentCases(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-dashboard-no-till")
	_, bank, _ := cashMovementTestAccounts(t, d, fx.orgID)
	now := time.UnixMilli(fx.date)

	set := func(accountID any) {
		t.Helper()
		if _, err := d.DB.Exec(`UPDATE organizations SET defaultCashRegisterAccountId = ? WHERE id = ?`, accountID, fx.orgID); err != nil {
			t.Fatalf("set register: %v", err)
		}
	}
	check := func(name string) {
		t.Helper()
		till, err := d.getDashboardCashRegister(fx.orgID, now, time.UTC)
		if err != nil || till != nil {
			t.Fatalf("%s: till=%+v err=%v, want nil, nil", name, till, err)
		}
	}

	set(nil)
	check("no register account")
	other := newGLPostingTestFixture(t, d, "org-dashboard-other")
	otherRegister, _, _ := cashMovementTestAccounts(t, d, other.orgID)
	set(otherRegister.ID)
	check("another organization's account")
	set(bank.ID)
	check("account never used")

	var group string
	if err := d.DB.Get(&group, `SELECT id FROM accounts WHERE organizationId = ? AND isGroup = 1 LIMIT 1`, fx.orgID); err == nil {
		set(group)
		check("group header account")
	}
}

func TestLoanFollowUpSelection(t *testing.T) {
	t.Parallel()
	stale := LoanStaleAfterDays
	from := LoanStaleAfterDays - LoanFollowUpWindowDays
	got := loanFollowUp(map[string]loanRegisterCustomer{
		"paid-up":     {ClientName: "Paid Up", Outstanding: 0, IdleDays: stale + 30},
		"recent":      {ClientName: "Recent", Outstanding: 500, IdleDays: from},
		"approaching": {ClientName: "Approaching", Outstanding: 700, IdleDays: from + 1},
		"at-limit":    {ClientName: "At Limit", Outstanding: 900, IdleDays: stale},
		"stale-small": {ClientName: "Stale Small", Outstanding: 100, IdleDays: stale + 5},
		"stale-big":   {ClientName: "Stale Big", Outstanding: 800, IdleDays: stale + 5},
		"oldest":      {ClientName: "Oldest", Outstanding: 50, IdleDays: stale + 40},
	})
	if got.StaleAfterDays != stale || got.StaleCount != 3 || got.ApproachingCount != 2 {
		t.Fatalf("counts = %+v, want 3 stale and 2 approaching", got)
	}
	want := []struct {
		id    string
		stale bool
	}{{"oldest", true}, {"stale-big", true}, {"stale-small", true}, {"at-limit", false}, {"approaching", false}}
	if len(got.Customers) != len(want) {
		t.Fatalf("customers = %+v, want %d", got.Customers, len(want))
	}
	for i, w := range want {
		if c := got.Customers[i]; c.ClientID != w.id || c.Stale != w.stale {
			t.Fatalf("customer %d = %+v, want %s (stale %v)", i, c, w.id, w.stale)
		}
	}

	many := map[string]loanRegisterCustomer{}
	for i := 0; i < loanFollowUpLimit+3; i++ {
		many[string(rune('a'+i))] = loanRegisterCustomer{Outstanding: 1, IdleDays: stale + 1 + i}
	}
	capped := loanFollowUp(many)
	if len(capped.Customers) != loanFollowUpLimit || capped.StaleCount != loanFollowUpLimit+3 {
		t.Fatalf("capped = %d customers, %d stale; want %d listed of %d", len(capped.Customers), capped.StaleCount, loanFollowUpLimit, loanFollowUpLimit+3)
	}
}

func TestGetLowStock(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-low-stock")
	if _, err := d.DB.Exec(`UPDATE organizations SET inventoryValuation = 'quantity_only' WHERE id = ?`, fx.orgID); err != nil {
		t.Fatalf("set quantity_only: %v", err)
	}
	stock := func(name string, qty float64) {
		t.Helper()
		p, err := d.CreateProduct(CreateProductRequest{OrganizationID: fx.orgID, Name: name, Type: "product", StockEnabled: 1})
		if err != nil {
			t.Fatalf("CreateProduct(%s): %v", name, err)
		}
		if qty > 0 {
			if _, err := d.CreateStockMovement(CreateStockMovementRequest{
				OrganizationID: fx.orgID, ProductID: p.ID, Type: "in", Quantity: qty,
			}); err != nil {
				t.Fatalf("CreateStockMovement(%s): %v", name, err)
			}
		}
	}
	stock("Plenty", 3)
	stock("Two left", 2)
	stock("One left", 1)
	stock("None", 0)
	if _, err := d.CreateProduct(CreateProductRequest{OrganizationID: fx.orgID, Name: "Service", Type: "service"}); err != nil {
		t.Fatalf("CreateProduct(service): %v", err)
	}

	low, err := d.getLowStock(fx.orgID)
	if err != nil {
		t.Fatalf("getLowStock: %v", err)
	}
	if low.Threshold != LowStockThreshold || low.Count != 3 || len(low.Items) != 3 {
		t.Fatalf("low = %+v, want 3 products at or below %v", low, LowStockThreshold)
	}
	for i, name := range []string{"None", "One left", "Two left"} {
		if low.Items[i].Name != name {
			t.Fatalf("item %d = %s, want %s (lowest first)", i, low.Items[i].Name, name)
		}
	}
}
