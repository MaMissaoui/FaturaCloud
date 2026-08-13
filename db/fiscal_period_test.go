package db

import "testing"

// TestCreateFiscalYearRejectsOverlapWithOpenYear is F52's regression test:
// a second open fiscal year spanning dates an existing open year already
// covers must be rejected, since resolveFiscalPeriodForDate would otherwise
// have an ambiguous choice of which year a posting on an overlapping date
// belongs to.
func TestCreateFiscalYearRejectsOverlapWithOpenYear(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-fy-overlap"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000, // 2025
	}); err != nil {
		t.Fatalf("CreateFiscalYear (2025): %v", err)
	}

	// Fully contained overlap.
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "overlap-contained", StartDate: 1738368000000, EndDate: 1740787199000,
	}); err == nil {
		t.Fatal("expected a fully-contained overlapping year to be rejected")
	}

	// Partial overlap at the tail end.
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "overlap-tail", StartDate: 1766225599000, EndDate: 1798761599000,
	}); err == nil {
		t.Fatal("expected a tail-overlapping year to be rejected")
	}

	// A genuinely adjacent, non-overlapping year is accepted.
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2026", StartDate: 1767225600000, EndDate: 1798761599000,
	}); err != nil {
		t.Fatalf("expected a non-overlapping adjacent year to succeed, got: %v", err)
	}

	// A different organization's identical range is unaffected.
	otherOrg, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-fy-overlap-other"})
	if err != nil {
		t.Fatalf("CreateOrganization (other): %v", err)
	}
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: otherOrg.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("expected the same range in a different org to succeed, got: %v", err)
	}
}

// TestCreateFiscalYearAllowsOverlapWithClosedYear covers the deliberate
// exception: a closed year no longer competes for postings
// (resolveFiscalPeriodForDate only ever matches status='open'), so a new
// year may legitimately overlap it — e.g. closing happens partway through
// the next year already having posted activity.
func TestCreateFiscalYearAllowsOverlapWithClosedYear(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-fy-overlap-closed")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	if _, err := d.CloseFiscalYear(fyID); err != nil {
		t.Fatalf("CloseFiscalYear: %v", err)
	}

	// The fixture's year is 2025 (1735689600000-1767225599000); this new one
	// overlaps it but is open, and the old one is now closed.
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: fx.orgID, Name: "2025-reopened-range", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("expected overlap with a closed year to be allowed, got: %v", err)
	}
}

// TestCreateFiscalPeriodRejectsRangeOutsideItsYear and
// TestCreateFiscalPeriodRejectsOverlapWithSiblingPeriod cover F52's period
// half: a period must fall inside its year's own range, and two periods in
// the same year must not overlap.
func TestCreateFiscalPeriodRejectsRangeOutsideItsYear(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-fp-outside"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	year, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	})
	if err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}

	// Starts before the year does.
	if _, err := d.CreateFiscalPeriod(CreateFiscalPeriodRequest{
		OrganizationID: org.ID, FiscalYearID: year.ID, Name: "Q0",
		StartDate: 1730000000000, EndDate: 1738368000000,
	}); err == nil {
		t.Fatal("expected a period starting before its year to be rejected")
	}

	// Ends after the year does.
	if _, err := d.CreateFiscalPeriod(CreateFiscalPeriodRequest{
		OrganizationID: org.ID, FiscalYearID: year.ID, Name: "Q5",
		StartDate: 1760000000000, EndDate: 1800000000000,
	}); err == nil {
		t.Fatal("expected a period ending after its year to be rejected")
	}

	// Fully inside is accepted.
	if _, err := d.CreateFiscalPeriod(CreateFiscalPeriodRequest{
		OrganizationID: org.ID, FiscalYearID: year.ID, Name: "Q1",
		StartDate: 1735689600000, EndDate: 1743465599000,
	}); err != nil {
		t.Fatalf("expected a period fully inside its year to succeed, got: %v", err)
	}
}

func TestCreateFiscalPeriodRejectsOverlapWithSiblingPeriod(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-fp-overlap"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	year, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	})
	if err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
	if _, err := d.CreateFiscalPeriod(CreateFiscalPeriodRequest{
		OrganizationID: org.ID, FiscalYearID: year.ID, Name: "Q1",
		StartDate: 1735689600000, EndDate: 1743465599000,
	}); err != nil {
		t.Fatalf("CreateFiscalPeriod (Q1): %v", err)
	}

	// Overlaps Q1's tail.
	if _, err := d.CreateFiscalPeriod(CreateFiscalPeriodRequest{
		OrganizationID: org.ID, FiscalYearID: year.ID, Name: "overlap",
		StartDate: 1740000000000, EndDate: 1746143999000,
	}); err == nil {
		t.Fatal("expected an overlapping sibling period to be rejected")
	}

	// A genuinely adjacent period is accepted.
	if _, err := d.CreateFiscalPeriod(CreateFiscalPeriodRequest{
		OrganizationID: org.ID, FiscalYearID: year.ID, Name: "Q2",
		StartDate: 1743465600000, EndDate: 1751327999000,
	}); err != nil {
		t.Fatalf("expected a non-overlapping adjacent period to succeed, got: %v", err)
	}
}

// TestResolveFiscalPeriodForDateIsDeterministicUnderOverlap guards the
// ORDER BY startDate DESC added to resolveFiscalPeriodForDate: even in a
// data anomaly where overlapping open years exist (pre-dating the
// CreateFiscalYear guard, or seeded directly), resolution must consistently
// pick the same one rather than depending on SQLite's unspecified row order.
func TestResolveFiscalPeriodForDateIsDeterministicUnderOverlap(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-fy-resolve-det"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	// Two overlapping open years, seeded directly (bypassing CreateFiscalYear's
	// guard) to simulate the pre-existing-anomaly case the ORDER BY defends.
	if _, err := d.DB.Exec(
		`INSERT INTO fiscal_years (id, organizationId, name, startDate, endDate, status) VALUES (?, ?, ?, ?, ?, 'open')`,
		"fy-early", org.ID, "early", 1735689600000, 1767225599000,
	); err != nil {
		t.Fatalf("seed fy-early: %v", err)
	}
	if _, err := d.DB.Exec(
		`INSERT INTO fiscal_years (id, organizationId, name, startDate, endDate, status) VALUES (?, ?, ?, ?, ?, 'open')`,
		"fy-late", org.ID, "late", 1738368000000, 1767225599000,
	); err != nil {
		t.Fatalf("seed fy-late: %v", err)
	}

	fiscalYearID, _, err := resolveFiscalPeriodForDate(d.DB, org.ID, 1740787200000)
	if err != nil {
		t.Fatalf("resolveFiscalPeriodForDate: %v", err)
	}
	if fiscalYearID != "fy-late" {
		t.Fatalf("resolved fiscal year = %q, want %q (most recently started open year)", fiscalYearID, "fy-late")
	}
	// Repeat to confirm it's not incidentally stable.
	for range 5 {
		got, _, err := resolveFiscalPeriodForDate(d.DB, org.ID, 1740787200000)
		if err != nil {
			t.Fatalf("resolveFiscalPeriodForDate: %v", err)
		}
		if got != "fy-late" {
			t.Fatalf("resolved fiscal year = %q, want %q (non-deterministic)", got, "fy-late")
		}
	}
}
