package db

import "testing"

// TestOrganizationIsTestFlag covers the isTest marker (migration 0099): set on
// create, cleared and set again through Update, and left alone when an
// update omits it.
func TestOrganizationIsTestFlag(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	plain, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-real"})
	if err != nil {
		t.Fatalf("CreateOrganization(real): %v", err)
	}
	if plain.IsTest {
		t.Fatalf("a new organization is a test organization by default")
	}

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-test", IsTest: ptr(true)})
	if err != nil {
		t.Fatalf("CreateOrganization(test): %v", err)
	}
	if !org.IsTest {
		t.Fatalf("isTest = false after create with isTest: true")
	}

	updated, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{IsTest: ptr(false)})
	if err != nil {
		t.Fatalf("clear isTest: %v", err)
	}
	if updated.IsTest {
		t.Fatalf("isTest = true after update with isTest: false")
	}

	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{IsTest: ptr(true)}); err != nil {
		t.Fatalf("set isTest: %v", err)
	}
	updated, err = d.UpdateOrganization(org.ID, UpdateOrganizationRequest{Name: ptr("Renamed")})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if !updated.IsTest {
		t.Fatalf("an update that omits isTest cleared it")
	}

	orgs, err := d.GetOrganizations()
	if err != nil {
		t.Fatalf("GetOrganizations: %v", err)
	}
	for _, o := range orgs {
		if o.IsTest != (o.ID == "org-test") {
			t.Fatalf("GetOrganizations: %s isTest = %v", o.ID, o.IsTest)
		}
	}
}
