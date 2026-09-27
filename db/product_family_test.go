package db

import (
	"errors"
	"strings"
	"testing"
)

func newFamilyTestOrg(t *testing.T, d *Database, orgID string) (string, *ProductFamily, *ProductFamily) {
	t.Helper()
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: orgID, Name: ptr("Family Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	washers, err := d.CreateProductFamily(CreateProductFamilyRequest{OrganizationID: org.ID, Name: "  Machine à laver  "})
	if err != nil {
		t.Fatalf("CreateProductFamily: %v", err)
	}
	fridges, err := d.CreateProductFamily(CreateProductFamilyRequest{OrganizationID: org.ID, Name: "Réfrigérateur"})
	if err != nil {
		t.Fatalf("CreateProductFamily: %v", err)
	}
	return org.ID, washers, fridges
}

func TestProductFamilyCRUD(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgID, washers, fridges := newFamilyTestOrg(t, d, "org-family-crud")

	if washers.Name != "Machine à laver" {
		t.Fatalf("name = %q, want trimmed", washers.Name)
	}
	var verr *ValidationError
	if _, err := d.CreateProductFamily(CreateProductFamilyRequest{OrganizationID: orgID, Name: "Réfrigérateur"}); !errors.As(err, &verr) {
		t.Fatalf("duplicate name: got %v, want a ValidationError", err)
	}
	if _, err := d.CreateProductFamily(CreateProductFamilyRequest{OrganizationID: orgID, Name: " "}); !errors.As(err, &verr) {
		t.Fatalf("blank name: got %v, want a ValidationError", err)
	}
	if _, err := d.UpdateProductFamily(fridges.ID, UpdateProductFamilyRequest{Name: ptr("Machine à laver")}); !errors.As(err, &verr) {
		t.Fatalf("rename onto an existing name: got %v, want a ValidationError", err)
	}
	renamed, err := d.UpdateProductFamily(fridges.ID, UpdateProductFamilyRequest{Name: ptr("Frigo")})
	if err != nil || renamed.Name != "Frigo" {
		t.Fatalf("rename = %+v, %v", renamed, err)
	}
	list, err := d.GetProductFamilies(orgID)
	if err != nil || len(list) != 2 || list[0].Name != "Frigo" {
		t.Fatalf("GetProductFamilies = %+v, %v (want 2, sorted by name)", list, err)
	}

	// Deleting a family in use only ungroups its products.
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: orgID, Name: "Washer", Type: "product", FamilyID: &washers.ID,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if ok, err := d.DeleteProductFamily(washers.ID); err != nil || !ok {
		t.Fatalf("DeleteProductFamily = %v, %v", ok, err)
	}
	p, err := d.GetProduct(product.ID)
	if err != nil || p.FamilyID != nil {
		t.Fatalf("product after family delete = familyId %v, err %v; want nil", p.FamilyID, err)
	}
}

// UpdateProduct is a full replace, except familyId: omitted keeps, "" clears,
// an id sets — so a caller that predates families can't wipe one.
func TestProductFamilyOnProductIsThreeStateAndOrgScoped(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgID, washers, fridges := newFamilyTestOrg(t, d, "org-family-product")
	otherOrg, otherFamily, _ := newFamilyTestOrg(t, d, "org-family-other")
	_ = otherOrg

	var verr *ValidationError
	if _, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: orgID, Name: "Cross-org", Type: "product", FamilyID: &otherFamily.ID,
	}); !errors.As(err, &verr) {
		t.Fatalf("cross-org family on create: got %v, want a ValidationError", err)
	}

	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: orgID, Name: "Washer", SKU: ptr("MAL-001"), Type: "product", FamilyID: &washers.ID,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	update := func(familyID *string) (*Product, error) {
		return d.UpdateProduct(product.ID, UpdateProductRequest{
			Name: "Washer", SKU: ptr("MAL-001"), Type: "product", FamilyID: familyID,
		})
	}

	if p, err := update(nil); err != nil || p.FamilyID == nil || *p.FamilyID != washers.ID {
		t.Fatalf("omitted familyId: got %v, %v; want kept", p.FamilyID, err)
	}
	if p, err := update(&fridges.ID); err != nil || p.FamilyID == nil || *p.FamilyID != fridges.ID {
		t.Fatalf("set familyId: got %v, %v", p.FamilyID, err)
	}
	if _, err := update(&otherFamily.ID); !errors.As(err, &verr) {
		t.Fatalf("cross-org family on update: got %v, want a ValidationError", err)
	}
	if p, err := update(ptr("")); err != nil || p.FamilyID != nil {
		t.Fatalf(`"" familyId: got %v, %v; want cleared`, p.FamilyID, err)
	}
}

func TestGetProductsFiltersByFamily(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgID, washers, fridges := newFamilyTestOrg(t, d, "org-family-filter")
	for i, fam := range []*string{&washers.ID, &washers.ID, &fridges.ID, nil} {
		if _, err := d.CreateProduct(CreateProductRequest{
			OrganizationID: orgID, Name: "P" + string(rune('A'+i)), Type: "product", FamilyID: fam,
		}); err != nil {
			t.Fatalf("CreateProduct: %v", err)
		}
	}
	for filter, want := range map[string]int{washers.ID: 2, fridges.ID: 1, "none": 1, "": 4} {
		_, total, err := d.GetProducts(orgID, ProductListOptions{FamilyID: filter})
		if err != nil || total != want {
			t.Fatalf("filter %q: total %d, err %v; want %d", filter, total, err, want)
		}
	}
}

func TestMassDataProductsFamilyColumn(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgID, washers, _ := newFamilyTestOrg(t, d, "org-family-excel")
	headers := productsMassDataSpec{}.Headers()
	if headers[productsColFamily] != "Family" {
		t.Fatalf("column %d = %q, want Family", productsColFamily, headers[productsColFamily])
	}
	row := func(id, name, family string) []string {
		r := make([]string, len(headers))
		r[0], r[1], r[3], r[4], r[productsColFamily] = id, name, "SKU-"+name, "product", family
		return r
	}

	res, err := d.ImportProductsXLSX(orgID, buildTestXLSX(t, headers, [][]string{
		row("", "Washer", "Machine à laver"),
		row("", "Mystery", "No such family"),
	}))
	if err != nil {
		t.Fatalf("ImportProductsXLSX: %v", err)
	}
	if res.Created != 1 || res.Failed != 1 || !strings.Contains(res.Rows[1].Error, "product family") {
		t.Fatalf("result = %+v, want 1 created and an unknown-family row error", res)
	}
	products, _, err := d.GetProducts(orgID, ProductListOptions{})
	if err != nil || len(products) != 1 || products[0].FamilyID == nil || *products[0].FamilyID != washers.ID {
		t.Fatalf("imported product family = %+v, %v", products, err)
	}
	id := products[0].ID

	// A 15-column sheet (exported before families) keeps the family.
	old := row(id, "Washer", "")[:productsColFamily]
	if _, err := d.ImportProductsXLSX(orgID, buildTestXLSX(t, headers[:productsColFamily], [][]string{old})); err != nil {
		t.Fatalf("ImportProductsXLSX (15 columns): %v", err)
	}
	if p, _ := d.GetProduct(id); p.FamilyID == nil || *p.FamilyID != washers.ID {
		t.Fatalf("15-column re-import cleared the family: %v", p.FamilyID)
	}

	// Export carries the name; a blank Family cell in a 16-column sheet clears it.
	content, err := d.ExportProductsXLSX(orgID)
	if err != nil {
		t.Fatalf("ExportProductsXLSX: %v", err)
	}
	rows, err := readXLSXRows(t, content)
	if err != nil || len(rows) != 2 || rows[1][productsColFamily] != "Machine à laver" {
		t.Fatalf("export rows = %v, %v", rows, err)
	}
	if _, err := d.ImportProductsXLSX(orgID, buildTestXLSX(t, headers, [][]string{row(id, "Washer", "")})); err != nil {
		t.Fatalf("ImportProductsXLSX (clear): %v", err)
	}
	if p, _ := d.GetProduct(id); p.FamilyID != nil {
		t.Fatalf("blank Family cell didn't clear: %v", *p.FamilyID)
	}
}
