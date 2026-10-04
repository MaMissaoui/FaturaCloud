package db

import "testing"

// TestOrganizationMasterDataSummaries covers the masterDataSummaries flag
// (migration 0100): defaults to true on create, persists through update,
// and is left alone when an update omits it.
func TestOrganizationMasterDataSummaries(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	// Default: true.
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-mds"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if !org.MasterDataSummaries {
		t.Fatal("masterDataSummaries should default to true")
	}

	// Clear to false.
	updated, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{MasterDataSummaries: ptr(false)})
	if err != nil {
		t.Fatalf("clear masterDataSummaries: %v", err)
	}
	if updated.MasterDataSummaries {
		t.Fatal("masterDataSummaries = true after update with false")
	}

	// Set back to true.
	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{MasterDataSummaries: ptr(true)}); err != nil {
		t.Fatalf("set masterDataSummaries: %v", err)
	}

	// An update that omits it leaves it unchanged.
	updated, err = d.UpdateOrganization(org.ID, UpdateOrganizationRequest{Name: ptr("Renamed")})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if !updated.MasterDataSummaries {
		t.Fatal("an update that omits masterDataSummaries cleared it")
	}
}
