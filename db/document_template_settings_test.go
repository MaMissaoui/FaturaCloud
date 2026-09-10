package db

import (
	"errors"
	"testing"
)

func TestDocumentTemplateOrientationDefaultsToNoOverride(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	orientation, err := d.GetDocumentTemplateOrientation(org.ID, "invoice")
	if err != nil {
		t.Fatalf("GetDocumentTemplateOrientation: %v", err)
	}
	if orientation != "" {
		t.Fatalf("got orientation %q, want \"\" (no override) before any is set", orientation)
	}
}

func TestSetDocumentTemplateOrientationRoundTrips(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	if err := d.SetDocumentTemplateOrientation(org.ID, "invoice", "landscape"); err != nil {
		t.Fatalf("SetDocumentTemplateOrientation: %v", err)
	}
	orientation, err := d.GetDocumentTemplateOrientation(org.ID, "invoice")
	if err != nil {
		t.Fatalf("GetDocumentTemplateOrientation: %v", err)
	}
	if orientation != "landscape" {
		t.Fatalf("got orientation %q, want landscape", orientation)
	}

	// A second document type on the same org is unaffected.
	other, err := d.GetDocumentTemplateOrientation(org.ID, "purchase_order")
	if err != nil {
		t.Fatalf("GetDocumentTemplateOrientation (purchase_order): %v", err)
	}
	if other != "" {
		t.Fatalf("got orientation %q for purchase_order, want \"\" (unaffected by invoice's setting)", other)
	}

	// Re-setting (the upsert path) overwrites rather than erroring.
	if err := d.SetDocumentTemplateOrientation(org.ID, "invoice", "portrait"); err != nil {
		t.Fatalf("SetDocumentTemplateOrientation (overwrite): %v", err)
	}
	orientation, err = d.GetDocumentTemplateOrientation(org.ID, "invoice")
	if err != nil {
		t.Fatalf("GetDocumentTemplateOrientation: %v", err)
	}
	if orientation != "portrait" {
		t.Fatalf("got orientation %q after overwrite, want portrait", orientation)
	}
}

func TestSetDocumentTemplateOrientationRejectsInvalidValue(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	err = d.SetDocumentTemplateOrientation(org.ID, "invoice", "sideways")
	if err == nil {
		t.Fatal("expected an invalid orientation value to be rejected")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
}

func TestDeleteDocumentTemplateOrientationRevertsToNoOverride(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	if err := d.SetDocumentTemplateOrientation(org.ID, "invoice", "landscape"); err != nil {
		t.Fatalf("SetDocumentTemplateOrientation: %v", err)
	}

	ok, err := d.DeleteDocumentTemplateOrientation(org.ID, "invoice")
	if err != nil {
		t.Fatalf("DeleteDocumentTemplateOrientation: %v", err)
	}
	if !ok {
		t.Fatal("expected DeleteDocumentTemplateOrientation to report a row was removed")
	}

	orientation, err := d.GetDocumentTemplateOrientation(org.ID, "invoice")
	if err != nil {
		t.Fatalf("GetDocumentTemplateOrientation: %v", err)
	}
	if orientation != "" {
		t.Fatalf("got orientation %q after delete, want \"\" (no override)", orientation)
	}

	// Deleting again (nothing left to delete) reports false, not an error.
	ok, err = d.DeleteDocumentTemplateOrientation(org.ID, "invoice")
	if err != nil {
		t.Fatalf("DeleteDocumentTemplateOrientation (second call): %v", err)
	}
	if ok {
		t.Fatal("expected the second delete to report false — nothing left to remove")
	}
}
