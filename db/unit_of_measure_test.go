package db

import "testing"

func newUnitOfMeasureTestOrg(t *testing.T) (d *Database, orgID string) {
	t.Helper()
	d = newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "uom-org"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	return d, org.ID
}

func TestCreateOrganizationSeedsDefaultUnitsOfMeasure(t *testing.T) {
	t.Parallel()
	d, orgID := newUnitOfMeasureTestOrg(t)

	units, err := d.GetUnitsOfMeasure(orgID)
	if err != nil {
		t.Fatalf("GetUnitsOfMeasure: %v", err)
	}
	if len(units) != len(defaultUnitsOfMeasure) {
		t.Fatalf("got %d seeded units, want %d", len(units), len(defaultUnitsOfMeasure))
	}
	var defaults int
	for _, u := range units {
		if u.IsDefault != nil && *u.IsDefault == 1 {
			defaults++
			if u.Name != "piece" {
				t.Errorf("default unit = %q, want %q", u.Name, "piece")
			}
		}
	}
	if defaults != 1 {
		t.Errorf("got %d default units, want exactly 1", defaults)
	}
}

func TestCreateUnitOfMeasureRejectsDuplicateName(t *testing.T) {
	t.Parallel()
	d, orgID := newUnitOfMeasureTestOrg(t)

	if _, err := d.CreateUnitOfMeasure(CreateUnitOfMeasureRequest{
		OrganizationID: orgID, Name: "Box of 12",
	}); err != nil {
		t.Fatalf("CreateUnitOfMeasure: %v", err)
	}
	if _, err := d.CreateUnitOfMeasure(CreateUnitOfMeasureRequest{
		OrganizationID: orgID, Name: "Box of 12",
	}); err == nil {
		t.Fatal("expected error for a duplicate name within the same organization")
	}
}

func TestUnitOfMeasureOnlyOneDefaultPerOrganization(t *testing.T) {
	t.Parallel()
	d, orgID := newUnitOfMeasureTestOrg(t)

	custom, err := d.CreateUnitOfMeasure(CreateUnitOfMeasureRequest{
		OrganizationID: orgID, Name: "Pallet", IsDefault: ptr(int64(1)),
	})
	if err != nil {
		t.Fatalf("CreateUnitOfMeasure: %v", err)
	}

	units, err := d.GetUnitsOfMeasure(orgID)
	if err != nil {
		t.Fatalf("GetUnitsOfMeasure: %v", err)
	}
	var defaults []string
	for _, u := range units {
		if u.IsDefault != nil && *u.IsDefault == 1 {
			defaults = append(defaults, u.Name)
		}
	}
	if len(defaults) != 1 || defaults[0] != "Pallet" {
		t.Fatalf("defaults = %v, want exactly [Pallet]", defaults)
	}

	got, err := d.GetUnitOfMeasure(custom.ID)
	if err != nil || got.IsDefault == nil || *got.IsDefault != 1 {
		t.Fatalf("GetUnitOfMeasure(custom) = %+v, %v; want isDefault=1", got, err)
	}
}

func TestUpdateUnitOfMeasureSwapsDefault(t *testing.T) {
	t.Parallel()
	d, orgID := newUnitOfMeasureTestOrg(t)

	units, err := d.GetUnitsOfMeasure(orgID)
	if err != nil {
		t.Fatalf("GetUnitsOfMeasure: %v", err)
	}
	var kgID string
	for _, u := range units {
		if u.Name == "kg" {
			kgID = u.ID
		}
	}
	if kgID == "" {
		t.Fatal("seeded units missing kg")
	}

	if _, err := d.UpdateUnitOfMeasure(kgID, UpdateUnitOfMeasureRequest{IsDefault: ptr(int64(1))}); err != nil {
		t.Fatalf("UpdateUnitOfMeasure: %v", err)
	}

	units, err = d.GetUnitsOfMeasure(orgID)
	if err != nil {
		t.Fatalf("GetUnitsOfMeasure: %v", err)
	}
	var defaultCount int
	for _, u := range units {
		if u.IsDefault != nil && *u.IsDefault == 1 {
			defaultCount++
			if u.Name != "kg" {
				t.Errorf("unexpected default %q still set", u.Name)
			}
		}
	}
	if defaultCount != 1 {
		t.Errorf("got %d defaults after swap, want 1", defaultCount)
	}
}

func TestDeleteUnitOfMeasureClearsProductLinkWithoutDeletingProduct(t *testing.T) {
	t.Parallel()
	d, orgID := newUnitOfMeasureTestOrg(t)

	units, err := d.GetUnitsOfMeasure(orgID)
	if err != nil {
		t.Fatalf("GetUnitsOfMeasure: %v", err)
	}
	var kgID string
	for _, u := range units {
		if u.Name == "kg" {
			kgID = u.ID
		}
	}

	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: orgID, Name: "Steel Bar", Type: "product", UnitOfMeasureID: &kgID,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if product.Unit == nil || *product.Unit != "kg" {
		t.Fatalf("product.Unit = %v, want denormalized %q", product.Unit, "kg")
	}
	if product.UnitOfMeasureID == nil || *product.UnitOfMeasureID != kgID {
		t.Fatalf("product.UnitOfMeasureID = %v, want %q", product.UnitOfMeasureID, kgID)
	}

	deleted, err := d.DeleteUnitOfMeasure(kgID)
	if err != nil || !deleted {
		t.Fatalf("DeleteUnitOfMeasure: deleted=%v err=%v", deleted, err)
	}

	after, err := d.GetProduct(product.ID)
	if err != nil {
		t.Fatalf("GetProduct after delete: %v", err)
	}
	if after.UnitOfMeasureID != nil {
		t.Errorf("UnitOfMeasureID = %v, want nil after deleting the unit of measure", after.UnitOfMeasureID)
	}
	if after.Unit == nil || *after.Unit != "kg" {
		t.Errorf("Unit = %v, want the last-resolved %q to survive as a display fallback", after.Unit, "kg")
	}
}

func TestCreateProductRejectsCrossOrgUnitOfMeasure(t *testing.T) {
	t.Parallel()
	d, orgID := newUnitOfMeasureTestOrg(t)
	otherOrg, err := d.CreateOrganization(CreateOrganizationRequest{ID: "uom-other-org"})
	if err != nil {
		t.Fatalf("CreateOrganization other: %v", err)
	}
	otherUnits, err := d.GetUnitsOfMeasure(otherOrg.ID)
	if err != nil || len(otherUnits) == 0 {
		t.Fatalf("GetUnitsOfMeasure other: %v, %d units", err, len(otherUnits))
	}

	_, err = d.CreateProduct(CreateProductRequest{
		OrganizationID: orgID, Name: "Cross-org test", Type: "product",
		UnitOfMeasureID: &otherUnits[0].ID,
	})
	if err == nil {
		t.Fatal("expected error creating a product with another organization's unit of measure")
	}
}

func TestUpdateProductWithoutUnitOfMeasureLeavesLegacyUnitAlone(t *testing.T) {
	t.Parallel()
	d, orgID := newUnitOfMeasureTestOrg(t)

	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: orgID, Name: "Legacy Widget", Type: "product", Unit: ptr("dozen"),
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if product.Unit == nil || *product.Unit != "dozen" {
		t.Fatalf("Unit = %v, want free-text %q preserved with no unitOfMeasureId set", product.Unit, "dozen")
	}
	if product.UnitOfMeasureID != nil {
		t.Fatalf("UnitOfMeasureID = %v, want nil", product.UnitOfMeasureID)
	}

	updated, err := d.UpdateProduct(product.ID, UpdateProductRequest{
		Name: "Legacy Widget", Type: "product", Unit: ptr("dozen still"),
	})
	if err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	if updated.Unit == nil || *updated.Unit != "dozen still" {
		t.Errorf("Unit = %v, want free-text update to still apply", updated.Unit)
	}
}
