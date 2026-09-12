package db

import "testing"

func TestCreateProductUppercasesAndTrimsSKU(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	sku := "  air-filt-9180  "
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Air filter", SKU: &sku,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if product.SKU == nil || *product.SKU != "AIR-FILT-9180" {
		t.Fatalf("got SKU %v, want AIR-FILT-9180", product.SKU)
	}
}

func TestCreateProductBlankSKUNormalizesToNil(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	blank := "   "
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Consulting", SKU: &blank,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if product.SKU != nil {
		t.Fatalf("got SKU %v, want nil (blank normalizes to no SKU)", *product.SKU)
	}
}

func TestUpdateProductUppercasesSKU(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	product, err := d.CreateProduct(CreateProductRequest{OrganizationID: org.ID, Name: "Widget"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	newSKU := "widget-42"
	updated, err := d.UpdateProduct(product.ID, UpdateProductRequest{Name: "Widget", SKU: &newSKU})
	if err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	if updated.SKU == nil || *updated.SKU != "WIDGET-42" {
		t.Fatalf("got SKU %v, want WIDGET-42", updated.SKU)
	}
}

// A SKU differing only by case must still collide with the unique index —
// normalizeSKU running before the INSERT/UPDATE is what makes this true;
// without it, "sku-1" and "SKU-1" would be two distinct rows.
func TestCreateProductDuplicateSKUDifferingOnlyByCase(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	first := "sku-1"
	if _, err := d.CreateProduct(CreateProductRequest{OrganizationID: org.ID, Name: "A", SKU: &first}); err != nil {
		t.Fatalf("CreateProduct (first): %v", err)
	}
	second := "SKU-1"
	_, err = d.CreateProduct(CreateProductRequest{OrganizationID: org.ID, Name: "B", SKU: &second})
	if err != ErrDuplicateSKU {
		t.Fatalf("got err %v, want ErrDuplicateSKU", err)
	}
}
