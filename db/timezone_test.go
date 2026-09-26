package db

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// The three instants a Tunis (UTC+1, no DST) organization actually stores
// for "20 February 2025": a date picked in a DatePicker (local midnight),
// a form's dayjs() default in the afternoon, and one just after midnight.
// Before organizations.timezone, each was read back as the wrong day on
// at least one path (UTC days for exports/buckets/numbers, +12h rounding for
// the e-invoice).
var (
	tunis              = mustLoadLocation("Africa/Tunis")
	berlin             = mustLoadLocation("Europe/Berlin")
	tunisPicked        = time.Date(2025, 2, 20, 0, 0, 0, 0, tunis).UnixMilli()
	tunisAfternoon     = time.Date(2025, 2, 20, 14, 37, 0, 0, tunis).UnixMilli()
	tunisAfterMidnight = time.Date(2025, 2, 20, 0, 30, 0, 0, tunis).UnixMilli()
)

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func setOrgTimezone(t *testing.T, d *Database, orgID, tz string) {
	t.Helper()
	if _, err := d.UpdateOrganization(orgID, UpdateOrganizationRequest{Timezone: &tz}); err != nil {
		t.Fatalf("UpdateOrganization(timezone=%q): %v", tz, err)
	}
}

func TestDateFormattingUsesOrganizationTimezone(t *testing.T) {
	t.Parallel()
	iso := ptr("YYYY-MM-DD")
	tz := ptr("Africa/Tunis")
	for name, ms := range map[string]int64{
		"picked":         tunisPicked,
		"afternoon":      tunisAfternoon,
		"after midnight": tunisAfterMidnight,
	} {
		if got := formatOrgDate(ms, iso, orgLocation(tz)); got != "2025-02-20" {
			t.Errorf("%s: formatOrgDate = %s, want 2025-02-20", name, got)
		}
		if got := formatMillis(ms, tz); got != "2025-02-20" {
			t.Errorf("%s: e-invoice formatMillis = %s, want 2025-02-20", name, got)
		}
		if got := formatFECDate(ms, orgLocation(tz)); got != "20250220" {
			t.Errorf("%s: formatFECDate = %s, want 20250220", name, got)
		}
		if got := formatDATEVBelegdatum(ms, orgLocation(tz)); got != "2002" {
			t.Errorf("%s: formatDATEVBelegdatum = %s, want 2002", name, got)
		}
	}
}

// With no zone set, every path keeps exactly its pre-0092 behaviour: UTC
// days, and the e-invoice's round-to-nearest-UTC-day heuristic.
func TestDateFormattingWithoutTimezoneKeepsPreviousBehaviour(t *testing.T) {
	t.Parallel()
	iso := ptr("YYYY-MM-DD")
	for _, tz := range []*string{nil, ptr("")} {
		if got := formatOrgDate(tunisPicked, iso, orgLocation(tz)); got != "2025-02-19" {
			t.Errorf("formatOrgDate = %s, want the UTC day 2025-02-19", got)
		}
		if got := formatMillis(tunisPicked, tz); got != "2025-02-20" {
			t.Errorf("e-invoice picked = %s, want 2025-02-20 (+12h rounding)", got)
		}
		if got := formatMillis(tunisAfternoon, tz); got != "2025-02-21" {
			t.Errorf("e-invoice afternoon = %s, want 2025-02-21 (the heuristic's known misread)", got)
		}
	}
}

func TestValidateTimezone(t *testing.T) {
	t.Parallel()
	for _, ok := range []*string{nil, ptr(""), ptr("Africa/Tunis"), ptr("Europe/Berlin"), ptr("UTC")} {
		if err := validateTimezone(ok); err != nil {
			t.Errorf("validateTimezone(%v) = %v, want nil", deref(ok), err)
		}
	}
	for _, bad := range []string{"Local", "Mars/Olympus_Mons", "UTC+1"} {
		err := validateTimezone(&bad)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("validateTimezone(%q) = %v, want a ValidationError", bad, err)
		}
	}
}

func TestOrganizationTimezoneSetAndClear(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-tz", Timezone: ptr("Africa/Tunis")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if deref(org.Timezone) != "Africa/Tunis" {
		t.Fatalf("created timezone = %q, want Africa/Tunis", deref(org.Timezone))
	}

	// Omitted keeps, "" clears, an invalid zone is refused.
	updated, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{Name: ptr("renamed")})
	if err != nil || deref(updated.Timezone) != "Africa/Tunis" {
		t.Fatalf("omitted timezone: got %q, err %v, want Africa/Tunis kept", deref(updated.Timezone), err)
	}
	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{Timezone: ptr("Nowhere/Void")}); err == nil {
		t.Fatal("expected an invalid time zone to be rejected")
	}
	updated, err = d.UpdateOrganization(org.ID, UpdateOrganizationRequest{Timezone: ptr("")})
	if err != nil || deref(updated.Timezone) != "" {
		t.Fatalf("cleared timezone: got %q, err %v, want empty", deref(updated.Timezone), err)
	}
	loc, err := organizationLocation(d.DB, org.ID)
	if err != nil || loc != time.UTC {
		t.Fatalf("cleared zone resolves to %v (err %v), want UTC", loc, err)
	}
}

// The Cash Book register: a backdated sale (picked date, local midnight)
// and a sale just after midnight both belong to 20 February, and the
// screen's UTC-noon day query finds them there, in both the daily summary
// and the per-transaction drill-down.
func TestDailyCashMovementsBucketByOrganizationDay(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-tz-cash")
	setOrgTimezone(t, d, fx.orgID, "Africa/Tunis")
	register, bank, _ := cashMovementTestAccounts(t, d, fx.orgID)

	postManualEntryForBalanceTest(t, d, fx.orgID, register.ID, bank.ID, 700, tunisPicked)
	if _, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: register.ID, Date: tunisAfterMidnight,
		CounterAccountType: "bank", CounterAccountID: bank.ID, Amount: 200,
	}); err != nil {
		t.Fatalf("CreateCashMovement: %v", err)
	}

	day := time.Date(2025, 2, 20, 12, 0, 0, 0, time.UTC).UnixMilli() // the screen's query
	rows, err := d.GetDailyCashMovements(fx.orgID, register.ID, day, day)
	if err != nil {
		t.Fatalf("GetDailyCashMovements: %v", err)
	}
	if len(rows) != 1 || rows[0].Date != "2025-02-20" || rows[0].Opening != 0 || rows[0].In != 700 || rows[0].Out != 200 {
		t.Fatalf("rows = %+v, want one 2025-02-20 row: opening 0, in 700, out 200", rows)
	}
	details, err := d.GetCashMovementDetails(fx.orgID, register.ID, day, day)
	if err != nil {
		t.Fatalf("GetCashMovementDetails: %v", err)
	}
	if len(details) != 1 || details[0].Amount != 200 {
		t.Fatalf("details = %+v, want the 00:30 withdrawal", details)
	}

	// The day before must be empty, not holding the two 20 February rows.
	prev := day - 86400000
	rows, err = d.GetDailyCashMovements(fx.orgID, register.ID, prev, prev)
	if err != nil {
		t.Fatalf("GetDailyCashMovements(prev): %v", err)
	}
	if len(rows) != 1 || rows[0].In != 0 || rows[0].Out != 0 {
		t.Fatalf("19 February = %+v, want no activity", rows)
	}
}

// Across Berlin's switch back from summer time (26 October 2025, a 25-hour
// day) the report still yields one row per calendar day, and a movement at
// 00:30 on the 27th (23:30 UTC on the 26th) lands on the 27th, where UTC
// bucketing would put it on the 26th.
func TestDailyCashMovementsAcrossDaylightSavingChange(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-tz-dst")
	setOrgTimezone(t, d, fx.orgID, "Europe/Berlin")
	register, bank, _ := cashMovementTestAccounts(t, d, fx.orgID)

	earlyOn27th := time.Date(2025, 10, 27, 0, 30, 0, 0, berlin).UnixMilli()
	postManualEntryForBalanceTest(t, d, fx.orgID, register.ID, bank.ID, 400, earlyOn27th)

	start := time.Date(2025, 10, 25, 12, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2025, 10, 27, 12, 0, 0, 0, time.UTC).UnixMilli()
	rows, err := d.GetDailyCashMovements(fx.orgID, register.ID, start, end)
	if err != nil {
		t.Fatalf("GetDailyCashMovements: %v", err)
	}
	var got []string
	for _, r := range rows {
		got = append(got, r.Date)
	}
	if len(rows) != 3 || got[0] != "2025-10-25" || got[1] != "2025-10-26" || got[2] != "2025-10-27" {
		t.Fatalf("days = %v, want 2025-10-25, -26, -27", got)
	}
	if rows[1].In != 0 || rows[2].In != 400 || rows[2].Opening != 0 {
		t.Fatalf("rows = %+v, want the 400 on 27 October only", rows)
	}
}

// A Tunis invoice dated 1 January (picked: local midnight, 23:00 UTC on
// 31 December) is January revenue and inside that year's dashboard range.
func TestRevenueByMonthAndYearRangeUseOrganizationTimezone(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-tz-rev", Currency: ptr("TND"), Timezone: ptr("Africa/Tunis")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	client, err := d.CreateClient(CreateClientRequest{OrganizationID: org.ID, Name: ptr("Client")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, tunis).UnixMilli()
	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-jan1", OrganizationID: org.ID, Number: "INV-1", State: "sent", ClientID: client.ID,
		Date: jan1, Currency: "TND", Total: 1000, SubTotal: 1000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 1000}},
	}); err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	start, end, err := d.DashboardYearRange(org.ID, 2026)
	if err != nil {
		t.Fatalf("DashboardYearRange: %v", err)
	}
	if start != jan1 {
		t.Fatalf("2026 starts at %v, want Tunis midnight %v", time.UnixMilli(start).UTC(), time.UnixMilli(jan1).UTC())
	}
	rows, err := d.GetRevenueByMonth(org.ID, start, end)
	if err != nil {
		t.Fatalf("GetRevenueByMonth: %v", err)
	}
	if len(rows) != 1 || rows[0].Month != "2026-01" || rows[0].Revenue != 1000 {
		t.Fatalf("rows = %+v, want one 2026-01 row with revenue 1000", rows)
	}
}

// The {day}/{month}/{year} tokens name the document's day in the
// organization's zone, whatever zone the server runs in.
func TestDocumentNumberDateTokensUseOrganizationTimezone(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-tz-num", Timezone: ptr("Africa/Tunis")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.UpdateDocumentNumberSetting(org.ID, "order", UpdateDocumentNumberSettingRequest{Format: "ORD-{year}{month}{day}-{number}"}); err != nil {
		t.Fatalf("UpdateDocumentNumberSetting: %v", err)
	}
	newYearsDay := time.Date(2026, 1, 1, 0, 0, 0, 0, tunis) // 23:00 UTC on 31 Dec 2025
	tx, err := d.DB.Beginx()
	if err != nil {
		t.Fatalf("Beginx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck
	got, err := GenerateNextDocumentNumberTx(tx, org.ID, "order", newYearsDay.In(time.UTC), "")
	if err != nil {
		t.Fatalf("GenerateNextDocumentNumberTx: %v", err)
	}
	if got != "ORD-20260101-1" {
		t.Fatalf("number = %s, want ORD-20260101-1", got)
	}
}

// TestMigration0092BackfillsTimezone steps back to 0091, inserts
// organizations, and re-applies 0092: single-zone countries get their zone
// by code or free-text name, everyone else stays NULL (UTC, as before).
func TestMigration0092BackfillsTimezone(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("migration source: %v", err)
	}
	dbDriver, err := sqlite.WithInstance(d.DB.DB, &sqlite.Config{})
	if err != nil {
		t.Fatalf("migration db driver: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", dbDriver)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}
	if err := m.Migrate(91); err != nil {
		t.Fatalf("migrate down to 0091: %v", err)
	}

	for _, org := range []struct{ id, country, countryCode string }{
		{"tn-code", "", " tn "},
		{"tn-name", "Tunisie", ""},
		{"de", "", "DE"},
		{"fr-name", "France", ""},
		{"at-name", "Österreich", ""},
		{"es", "Spain", "ES"}, // two zones (Canary Islands): left unset
		{"us", "United States", "US"},
		{"blank", "", ""},
	} {
		if _, err := d.DB.Exec(
			`INSERT INTO organizations (id, name, country, country_code) VALUES (?, ?, ?, ?)`,
			org.id, org.id, org.country, org.countryCode,
		); err != nil {
			t.Fatalf("insert %s: %v", org.id, err)
		}
	}

	if err := m.Migrate(92); err != nil {
		t.Fatalf("migrate up to 0092: %v", err)
	}

	want := map[string]*string{
		"tn-code": ptr("Africa/Tunis"),
		"tn-name": ptr("Africa/Tunis"),
		"de":      ptr("Europe/Berlin"),
		"fr-name": ptr("Europe/Paris"),
		"at-name": ptr("Europe/Vienna"),
		"es":      nil,
		"us":      nil,
		"blank":   nil,
	}
	for id, w := range want {
		var got *string
		if err := d.DB.Get(&got, `SELECT timezone FROM organizations WHERE id = ?`, id); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if (got == nil) != (w == nil) || (got != nil && *got != *w) {
			t.Errorf("%s: timezone = %v, want %v", id, deref(got), deref(w))
		}
		if got != nil {
			if _, err := time.LoadLocation(*got); err != nil {
				t.Errorf("%s: backfilled zone %q doesn't load: %v", id, *got, err)
			}
		}
	}
}

// Every zone 0092 can backfill must load, or orgLocation would silently
// fall back to UTC for exactly the organizations the backfill targeted.
func TestMigration0092ZonesAllLoad(t *testing.T) {
	t.Parallel()
	sql, err := migrationsFS.ReadFile("migrations/0092_add_organization_timezone.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	zones := regexp.MustCompile(`THEN '([^']+)'`).FindAllStringSubmatch(string(sql), -1)
	if len(zones) < 10 {
		t.Fatalf("found %d zones in 0092, expected the full backfill list", len(zones))
	}
	for _, z := range zones {
		if _, err := time.LoadLocation(z[1]); err != nil {
			t.Errorf("0092 backfills %q, which doesn't load: %v", z[1], err)
		}
	}
}
