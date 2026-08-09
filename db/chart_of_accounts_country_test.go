package db

import (
	"regexp"
	"testing"
)

// TestCreateOrganizationWithGermanyCountrySeedsSKR04Chart confirms an
// organization created with Country "Germany" gets skr04ChartOfAccounts
// (real SKR04 numbers) instead of the generic starter chart, with every
// default*AccountId wired to the matching SKR04 leaf and every leaf's code
// carrying a datevAccountNumber — the property db/export_datev.go's
// Sachkontenlänge rule depends on.
func TestCreateOrganizationWithGermanyCountrySeedsSKR04Chart(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: "org-de-skr04", Name: ptr("Deutsche GmbH"), Country: ptr("Germany"),
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	accounts, err := d.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	byCode := map[string]Account{}
	for _, a := range accounts {
		byCode[a.Code] = a
	}

	// A handful of real, cited SKR04 numbers must be present with the right type.
	for code, wantType := range map[string]string{
		"1600": "asset", "1800": "asset", "1200": "asset", "1400": "asset",
		"3300": "liability", "3800": "liability",
		"2970": "equity",
		"4400": "revenue", "4840": "revenue",
		"5000": "expense", "6880": "expense",
	} {
		a, ok := byCode[code]
		if !ok {
			t.Fatalf("expected SKR04 account %s to be seeded", code)
		}
		if a.Type != wantType {
			t.Fatalf("account %s: expected type %s, got %s", code, wantType, a.Type)
		}
	}

	// Every skr04ChartOfAccounts leaf's code doubles as its datevAccountNumber
	// and is uniformly 4 digits — the exact property db/export_datev.go's
	// Sachkontenlänge check requires; a mixed-length DE chart must never ship.
	// phase7BaseAdditions (1150/2150/5150/5900) are deliberately excluded: per
	// the design comment on phase7BaseAdditions, no single real SKR04 number
	// exists for those app-specific accounts, so they seed with no
	// datevAccountNumber even for a German org — same as an organization
	// would need to configure manually today for any account DATEV export
	// needs, per CLAUDE.md's Chart of Accounts form note.
	fourDigits := regexp.MustCompile(`^\d{4}$`)
	for _, a := range skr04ChartOfAccounts {
		if a.isGroup {
			continue
		}
		seeded, ok := byCode[a.code]
		if !ok {
			t.Fatalf("expected SKR04 template leaf %s to be seeded", a.code)
		}
		if !fourDigits.MatchString(seeded.Code) {
			t.Fatalf("SKR04 leaf account code %q is not a uniform 4-digit code", seeded.Code)
		}
		if seeded.DATEVAccountNumber == nil || *seeded.DATEVAccountNumber != seeded.Code {
			t.Fatalf("SKR04 leaf account %s: expected datevAccountNumber == code, got %v", seeded.Code, seeded.DATEVAccountNumber)
		}
	}
	for _, code := range []string{"1150", "2150", "5150", "5900"} {
		if a := byCode[code]; a.DATEVAccountNumber != nil {
			t.Fatalf("phase7 account %s: expected no datevAccountNumber even for a DE org, got %v", code, *a.DATEVAccountNumber)
		}
	}

	// The core defaults resolve to the real SKR04 leaves, not the generic chart's.
	mustWire := map[string]*string{
		"1800 (Bank -> cash)":        org.DefaultCashAccountID,
		"1200 (Forderungen -> AR)":   org.DefaultArAccountID,
		"3300 (Verb. -> AP)":         org.DefaultApAccountID,
		"4400 (Erlöse -> revenue)":   org.DefaultRevenueAccountID,
		"5000 (Aufw. -> expense)":    org.DefaultExpenseAccountID,
		"2970 (Gewinnvortrag -> RE)": org.RetainedEarningsAccountID,
		"4840 (-> fxGain)":           org.FxGainAccountID,
		"6880 (-> fxLoss)":           org.FxLossAccountID,
	}
	for label, id := range mustWire {
		if id == nil {
			t.Fatalf("expected %s to be wired, got nil", label)
		}
	}
	if *org.DefaultCashAccountID != byCode["1800"].ID {
		t.Fatalf("expected defaultCashAccountId to resolve to SKR04 1800 Bank")
	}
	if *org.DefaultArAccountID != byCode["1200"].ID {
		t.Fatalf("expected defaultArAccountId to resolve to SKR04 1200")
	}
	if *org.DefaultApAccountID != byCode["3300"].ID {
		t.Fatalf("expected defaultApAccountId to resolve to SKR04 3300")
	}

	// Phase 7's inventory/COGS accounts stay the shared, honestly-generic
	// codes (not a fabricated SKR04 number), parented under whichever SKR04
	// header matches their type instead of the generic chart's 1000/2000/5000.
	inventory, ok := byCode["1150"]
	if !ok || org.DefaultInventoryAccountID == nil || inventory.ID != *org.DefaultInventoryAccountID {
		t.Fatalf("expected 1150 Inventory wired as defaultInventoryAccountId")
	}
	assetHeader, ok := byCode["1"]
	if !ok || !boolFromIsGroup(assetHeader) || assetHeader.Type != "asset" {
		t.Fatalf("expected SKR04 asset header at code \"1\"")
	}
	if inventory.ParentID == nil || *inventory.ParentID != assetHeader.ID {
		t.Fatalf("expected Phase 7 Inventory (1150) parented under the SKR04 asset header, not a generic-chart code")
	}
}

// TestCreateOrganizationWithFranceCountrySeedsPCGChart mirrors the SKR04
// test above for pcgChartOfAccounts: real PCG numbers, correct default
// wiring, and — since France has no DATEV — no datevAccountNumber anywhere.
func TestCreateOrganizationWithFranceCountrySeedsPCGChart(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: "org-fr-pcg", Name: ptr("Société Française"), Country: ptr("France"),
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	accounts, err := d.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	byCode := map[string]Account{}
	for _, a := range accounts {
		byCode[a.Code] = a
		if a.DATEVAccountNumber != nil {
			t.Fatalf("PCG account %s: expected no datevAccountNumber (France has no DATEV), got %v", a.Code, *a.DATEVAccountNumber)
		}
	}

	for code, wantType := range map[string]string{
		"411": "asset", "512": "asset", "53": "asset", "4456": "asset",
		"401": "liability", "4457": "liability",
		"110": "equity",
		"70":  "revenue", "766": "revenue",
		"60": "expense", "666": "expense",
	} {
		a, ok := byCode[code]
		if !ok {
			t.Fatalf("expected PCG account %s to be seeded", code)
		}
		if a.Type != wantType {
			t.Fatalf("account %s: expected type %s, got %s", code, wantType, a.Type)
		}
	}

	if org.DefaultArAccountID == nil || *org.DefaultArAccountID != byCode["411"].ID {
		t.Fatalf("expected defaultArAccountId to resolve to PCG 411 Clients")
	}
	if org.DefaultApAccountID == nil || *org.DefaultApAccountID != byCode["401"].ID {
		t.Fatalf("expected defaultApAccountId to resolve to PCG 401 Fournisseurs")
	}
	if org.DefaultCashAccountID == nil || *org.DefaultCashAccountID != byCode["512"].ID {
		t.Fatalf("expected defaultCashAccountId to resolve to PCG 512 Banques")
	}

	cogs, ok := byCode["5150"]
	if !ok || org.DefaultCOGSAccountID == nil || cogs.ID != *org.DefaultCOGSAccountID {
		t.Fatalf("expected 5150 Cost of Goods Sold wired as defaultCOGSAccountId")
	}
	expenseHeader, ok := byCode["5000"]
	if !ok || !boolFromIsGroup(expenseHeader) || expenseHeader.Type != "expense" {
		t.Fatalf("expected PCG expense header at code \"5000\"")
	}
	if cogs.ParentID == nil || *cogs.ParentID != expenseHeader.ID {
		t.Fatalf("expected Phase 7 COGS (5150) parented under the PCG expense header")
	}
}

// TestCreateOrganizationWithUnmappedCountryKeepsGenericChart is the
// regression guard for chartTemplates' fallback: a country that isn't a
// key in chartTemplates (or no country at all) must still get the same
// generic starter chart every organization got before this feature existed.
func TestCreateOrganizationWithUnmappedCountryKeepsGenericChart(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: "org-es-generic", Name: ptr("Empresa Española"), Country: ptr("Spain"),
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	accounts, err := d.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	byCode := map[string]Account{}
	for _, a := range accounts {
		byCode[a.Code] = a
	}

	if _, ok := byCode["1100"]; !ok {
		t.Fatalf("expected the generic chart's 1100 Accounts Receivable for an unmapped country")
	}
	if _, ok := byCode["1600"]; ok {
		t.Fatalf("did not expect SKR04's 1600 (Kasse) to leak into an unmapped-country organization")
	}
	if _, ok := byCode["411"]; ok {
		t.Fatalf("did not expect PCG's 411 to leak into an unmapped-country organization")
	}
}

func boolFromIsGroup(a Account) bool { return a.IsGroup == 1 }
