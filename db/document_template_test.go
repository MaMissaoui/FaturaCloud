package db

import (
	"bytes"
	"testing"
)

// TestResolveTemplateBytesFollowsDocumentLayout covers the documentLayout
// selector: with no override, "tunisia" picks the Tunisian embedded template
// and anything else (NULL, "", "default", an unrecognized value) falls back
// to the generic default; an uploaded override wins regardless of layout.
func TestResolveTemplateBytesFollowsDocumentLayout(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	for _, tc := range []struct {
		name   string
		layout *string
		want   []byte
		source string
	}{
		{"nil", nil, invoiceDefaultTemplate, DocumentLayoutDefault},
		{"empty", ptr(""), invoiceDefaultTemplate, DocumentLayoutDefault},
		{"default", ptr("default"), invoiceDefaultTemplate, DocumentLayoutDefault},
		{"unrecognized", ptr("custom"), invoiceDefaultTemplate, DocumentLayoutDefault},
		{"tunisia", ptr("tunisia"), invoiceTunisiaTemplate, DocumentLayoutTunisia},
	} {
		got, source, err := resolveTemplateBytes(d, org.ID, "invoice", tc.layout)
		if err != nil {
			t.Fatalf("%s: resolveTemplateBytes: %v", tc.name, err)
		}
		if !bytes.Equal(got, tc.want) || source != tc.source {
			t.Errorf("%s: got source %q, want %q (or wrong bytes)", tc.name, source, tc.source)
		}
	}

	custom := buildFixtureTemplate(t)
	if err := d.UploadDocumentTemplate(org.ID, "invoice", "mine.xlsx", custom); err != nil {
		t.Fatalf("UploadDocumentTemplate: %v", err)
	}
	for _, layout := range []*string{nil, ptr("default"), ptr("tunisia")} {
		got, source, err := resolveTemplateBytes(d, org.ID, "invoice", layout)
		if err != nil {
			t.Fatalf("resolveTemplateBytes with override: %v", err)
		}
		if !bytes.Equal(got, custom) || source != "override" {
			t.Errorf("layout %v: expected the uploaded override to win, got source %q", layout, source)
		}
	}
}

// TestExportDocumentTemplateBytesFollowsDocumentLayout checks the template
// download endpoint's backing function reads the org's stored documentLayout,
// so "download the current template" matches what an export would fill.
func TestExportDocumentTemplateBytesFollowsDocumentLayout(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	content, filename, err := d.ExportDocumentTemplateBytes(org.ID, "purchase_order")
	if err != nil {
		t.Fatalf("ExportDocumentTemplateBytes (default): %v", err)
	}
	if filename != "purchase_order_default.xlsx" || !bytes.Equal(content, purchaseOrderDefaultTemplate) {
		t.Fatalf("expected the default-layout template, got %q", filename)
	}

	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{DocumentLayout: ptr("tunisia")}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	content, filename, err = d.ExportDocumentTemplateBytes(org.ID, "purchase_order")
	if err != nil {
		t.Fatalf("ExportDocumentTemplateBytes (tunisia): %v", err)
	}
	if filename != "purchase_order_tunisia.xlsx" || !bytes.Equal(content, purchaseOrderTunisiaTemplate) {
		t.Fatalf("expected the tunisia-layout template, got %q", filename)
	}

	// Switching back to "default" must actually take effect — the update SQL's
	// COALESCE only keeps the old value for a nil field, not an explicit one.
	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{DocumentLayout: ptr("default")}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	_, filename, err = d.ExportDocumentTemplateBytes(org.ID, "purchase_order")
	if err != nil || filename != "purchase_order_default.xlsx" {
		t.Fatalf("expected a switch back to default to take effect: filename=%q err=%v", filename, err)
	}
}

// TestCreateOrganizationStoresDocumentLayout guards the create path: a layout
// picked in the New Organization drawer must reach the stored row.
func TestCreateOrganizationStoresDocumentLayout(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1", DocumentLayout: ptr("tunisia")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if org.DocumentLayout == nil || *org.DocumentLayout != "tunisia" {
		t.Fatalf("expected documentLayout \"tunisia\", got %v", org.DocumentLayout)
	}
}
